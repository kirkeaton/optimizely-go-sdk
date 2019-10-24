/****************************************************************************
 * Copyright 2019, Optimizely, Inc. and contributors                        *
 *                                                                          *
 * Licensed under the Apache License, Version 2.0 (the "License");          *
 * you may not use this file except in compliance with the License.         *
 * You may obtain a copy of the License at                                  *
 *                                                                          *
 *    http://www.apache.org/licenses/LICENSE-2.0                            *
 *                                                                          *
 * Unless required by applicable law or agreed to in writing, software      *
 * distributed under the License is distributed on an "AS IS" BASIS,        *
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. *
 * See the License for the specific language governing permissions and      *
 * limitations under the License.                                           *
 ***************************************************************************/

package client

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/optimizely/go-sdk/pkg"
	"github.com/optimizely/go-sdk/pkg/decision"
	"github.com/optimizely/go-sdk/pkg/entities"
	"github.com/optimizely/go-sdk/pkg/event"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

var exeCtxSignalFlag bool

type ExecutionCtx struct {
	Wg  *sync.WaitGroup
	Ctx context.Context
}

func (ctx ExecutionCtx) TerminateAndWait() {
	exeCtxSignalFlag = true
}

func (ctx ExecutionCtx) GetContext() context.Context {
	return ctx.Ctx
}

func (ctx ExecutionCtx) GetWaitSync() *sync.WaitGroup {
	return ctx.Wg
}

func ValidProjectConfigManager() *MockProjectConfigManager {
	p := new(MockProjectConfigManager)
	p.projectConfig = new(TestConfig)
	return p
}

type MockProcessor struct {
	Events []event.UserEvent
}

func (f *MockProcessor) ProcessEvent(event event.UserEvent) {
	f.Events = append(f.Events, event)
}

type TestConfig struct {
	pkg.ProjectConfig
}

func (TestConfig) GetEventByKey(key string) (entities.Event, error) {
	if key == "sample_conversion" {
		return entities.Event{ExperimentIds: []string{"15402980349"}, ID: "15368860886", Key: "sample_conversion"}, nil
	}

	return entities.Event{}, errors.New("No conversion")
}

func (TestConfig) GetFeatureByKey(string) (entities.Feature, error) {
	return entities.Feature{}, nil
}

func (TestConfig) GetProjectID() string {
	return "15389410617"
}
func (TestConfig) GetRevision() string {
	return "7"
}
func (TestConfig) GetAccountID() string {
	return "8362480420"
}
func (TestConfig) GetAnonymizeIP() bool {
	return true
}
func (TestConfig) GetAttributeID(key string) string { // returns "" if there is no id
	return ""
}
func (TestConfig) GetBotFiltering() bool {
	return false
}
func (TestConfig) GetClientName() string {
	return "go-sdk"
}
func (TestConfig) GetClientVersion() string {
	return "1.0.0"
}

func TestTrack(t *testing.T) {
	mockProcessor := &MockProcessor{}
	mockDecisionService := new(MockDecisionService)

	client := OptimizelyClient{
		ConfigManager:   ValidProjectConfigManager(),
		DecisionService: mockDecisionService,
		EventProcessor:  mockProcessor,
	}

	err := client.Track("sample_conversion", entities.UserContext{ID: "1212121", Attributes: map[string]interface{}{}}, map[string]interface{}{})

	assert.Nil(t, err)
	assert.True(t, len(mockProcessor.Events) == 1)
	assert.True(t, mockProcessor.Events[0].VisitorID == "1212121")
	assert.True(t, mockProcessor.Events[0].EventContext.ProjectID == "15389410617")

}

func TestTrackFailEventNotFound(t *testing.T) {
	mockProcessor := &MockProcessor{}
	mockDecisionService := new(MockDecisionService)

	client := OptimizelyClient{
		ConfigManager:   ValidProjectConfigManager(),
		DecisionService: mockDecisionService,
		EventProcessor:  mockProcessor,
	}

	err := client.Track("bob", entities.UserContext{ID: "1212121", Attributes: map[string]interface{}{}}, map[string]interface{}{})

	assert.NoError(t, err)
	assert.True(t, len(mockProcessor.Events) == 0)

}

func TestTrackPanics(t *testing.T) {
	mockProcessor := &MockProcessor{}
	mockDecisionService := new(MockDecisionService)

	client := OptimizelyClient{
		ConfigManager:   new(PanickingConfigManager),
		DecisionService: mockDecisionService,
		EventProcessor:  mockProcessor,
	}

	err := client.Track("bob", entities.UserContext{ID: "1212121", Attributes: map[string]interface{}{}}, map[string]interface{}{})

	assert.Error(t, err)
	assert.True(t, len(mockProcessor.Events) == 0)

}

func TestGetEnabledFeaturesPanic(t *testing.T) {
	testUserContext := entities.UserContext{ID: "test_user_1"}
	mockDecisionService := new(MockDecisionService)

	client := OptimizelyClient{
		ConfigManager:   &PanickingConfigManager{},
		DecisionService: mockDecisionService,
	}

	// ensure that the client calms back down and recovers
	result, err := client.GetEnabledFeatures(testUserContext)
	assert.Empty(t, result)
	assert.True(t, assert.Error(t, err))
}

func TestGetFeatureVariableBooleanWithValidValue(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "true"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "false",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Boolean,
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, _ := client.GetFeatureVariableBoolean(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, true, result)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableBooleanWithInvalidValue(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "stringvalue"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "false",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Boolean,
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableBoolean(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, false, result)
	assert.Error(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableBooleanWithInvalidValueType(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "5"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "4",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Integer,
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableBoolean(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, false, result)
	assert.Error(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableBooleanWithEmptyValueType(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "5"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "4",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         "",
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableBoolean(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, false, result)
	assert.Error(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableBooleanReturnsDefaultValueIfFeatureNotEnabled(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "true"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "false",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Boolean,
	}
	testVariation := getTestVariationWithFeatureVariable(false, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableBoolean(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, false, result)
	assert.Nil(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableBoolPanic(t *testing.T) {
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_variable_key"

	mockDecisionService := new(MockDecisionService)

	client := OptimizelyClient{
		ConfigManager:   &PanickingConfigManager{},
		DecisionService: mockDecisionService,
	}

	// ensure that the client calms back down and recovers
	result, err := client.GetFeatureVariableBoolean(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, false, result)
	assert.True(t, assert.Error(t, err))
}

func TestGetFeatureVariableDoubleWithValidValue(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "5"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "4",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Double,
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, _ := client.GetFeatureVariableDouble(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, float64(5), result)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableDoubleWithInvalidValue(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "stringvalue"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "4",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Double,
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableDouble(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, float64(0), result)
	assert.Error(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableDoubleWithInvalidValueType(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "5"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "4",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Integer,
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableDouble(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, float64(0), result)
	assert.Error(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableDoubleWithEmptyValueType(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "5"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "4",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         "",
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableDouble(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, float64(0), result)
	assert.Error(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableDoubleReturnsDefaultValueIfFeatureNotEnabled(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "5"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "4",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Double,
	}
	testVariation := getTestVariationWithFeatureVariable(false, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableDouble(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, float64(4), result)
	assert.Nil(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableDoublePanic(t *testing.T) {
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_variable_key"

	mockDecisionService := new(MockDecisionService)

	client := OptimizelyClient{
		ConfigManager:   &PanickingConfigManager{},
		DecisionService: mockDecisionService,
	}

	// ensure that the client calms back down and recovers
	result, err := client.GetFeatureVariableDouble(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, float64(0), result)
	assert.True(t, assert.Error(t, err))
}

func TestGetFeatureVariableIntegerWithValidValue(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "5"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "4",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Integer,
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, _ := client.GetFeatureVariableInteger(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, 5, result)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableIntegerWithInvalidValue(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "stringvalue"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "4",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Integer,
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableInteger(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, 0, result)
	assert.Error(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableIntegerWithInvalidValueType(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "true"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "false",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Boolean,
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableInteger(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, 0, result)
	assert.Error(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableIntegerWithEmptyValueType(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "true"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "false",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         "",
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableInteger(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, 0, result)
	assert.Error(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableIntegerReturnsDefaultValueIfFeatureNotEnabled(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "5"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "4",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Integer,
	}
	testVariation := getTestVariationWithFeatureVariable(false, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableInteger(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, 4, result)
	assert.Nil(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableIntegerPanic(t *testing.T) {
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_variable_key"

	mockDecisionService := new(MockDecisionService)

	client := OptimizelyClient{
		ConfigManager:   &PanickingConfigManager{},
		DecisionService: mockDecisionService,
	}

	// ensure that the client calms back down and recovers
	result, err := client.GetFeatureVariableInteger(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, 0, result)
	assert.True(t, assert.Error(t, err))
}

func TestGetFeatureVariableStringWithValidValue(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "teststring"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "default",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.String,
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, _ := client.GetFeatureVariableString(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, testVariableValue, result)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableStringWithInvalidValueType(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "true"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "default",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.Boolean,
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableString(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, "", result)
	assert.Error(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableStringWithEmptyValueType(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "true"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "default",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         "",
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableString(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, "", result)
	assert.Error(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableStringReturnsDefaultValueIfFeatureNotEnabled(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "teststring"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "defaultString",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.String,
	}
	testVariation := getTestVariationWithFeatureVariable(false, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	result, err := client.GetFeatureVariableString(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, "defaultString", result)
	assert.Nil(t, err)
	mockConfig.AssertExpectations(t)
	mockConfigManager.AssertExpectations(t)
	mockDecisionService.AssertExpectations(t)
}

func TestGetFeatureVariableStringPanic(t *testing.T) {
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_variable_key"

	mockDecisionService := new(MockDecisionService)

	client := OptimizelyClient{
		ConfigManager:   &PanickingConfigManager{},
		DecisionService: mockDecisionService,
	}

	// ensure that the client calms back down and recovers
	result, err := client.GetFeatureVariableString(testFeatureKey, testVariableKey, testUserContext)
	assert.Equal(t, "", result)
	assert.True(t, assert.Error(t, err))
}

func TestGetFeatureVariableErrorCases(t *testing.T) {
	testUserContext := entities.UserContext{ID: "test_user_1"}

	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(nil, errors.New("no project config available"))
	mockDecisionService := new(MockDecisionService)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}
	_, err1 := client.GetFeatureVariableBoolean("test_feature_key", "test_variable_key", testUserContext)
	_, err2 := client.GetFeatureVariableDouble("test_feature_key", "test_variable_key", testUserContext)
	_, err3 := client.GetFeatureVariableInteger("test_feature_key", "test_variable_key", testUserContext)
	_, err4 := client.GetFeatureVariableString("test_feature_key", "test_variable_key", testUserContext)
	assert.Error(t, err1)
	assert.Error(t, err2)
	assert.Error(t, err3)
	assert.Error(t, err4)
	mockConfigManager.AssertNotCalled(t, "GetFeatureByKey")
	mockConfigManager.AssertNotCalled(t, "GetVariableByKey")
	mockDecisionService.AssertNotCalled(t, "GetFeatureDecision")
}

func TestGetProjectConfigIsValid(t *testing.T) {
	mockConfigManager := ValidProjectConfigManager()

	client := OptimizelyClient{
		ConfigManager: mockConfigManager,
	}

	actual, err := client.GetProjectConfig()

	assert.Nil(t, err)
	assert.Equal(t, mockConfigManager.projectConfig, actual)
}

func TestGetFeatureDecisionValid(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "teststring"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "defaultString",
		ID:           "1",
		Key:          "test_feature_flag_key",
		Type:         entities.String,
	}
	testVariation := getTestVariationWithFeatureVariable(false, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}

	_, featureDecision, err := client.getFeatureDecision(testFeatureKey, testUserContext)
	assert.Nil(t, err)
	assert.Equal(t, expectedFeatureDecision, featureDecision)
}

func TestGetFeatureDecisionErrProjectConfig(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "teststring"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "defaultString",
		ID:           "1",
		Key:          testVariableKey,
		Type:         entities.String,
	}
	testVariation := getTestVariationWithFeatureVariable(false, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, errors.New("project config error"))

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}

	_, _, err := client.getFeatureDecision(testFeatureKey, testUserContext)
	assert.Error(t, err)
}

func TestGetFeatureDecisionPanicProjectConfig(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "teststring"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "defaultString",
		ID:           "1",
		Key:          testVariableKey,
		Type:         entities.String,
	}
	testVariation := getTestVariationWithFeatureVariable(false, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)

	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   &PanickingConfigManager{},
		DecisionService: mockDecisionService,
	}

	_, _, err := client.getFeatureDecision(testFeatureKey, testUserContext)
	assert.Error(t, err)
}

func TestGetFeatureDecisionPanicDecisionService(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "teststring"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "defaultString",
		ID:           "1",
		Key:          testVariableKey,
		Type:         entities.String,
	}
	testVariation := getTestVariationWithFeatureVariable(false, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: &PanickingDecisionService{},
	}

	_, _, err := client.getFeatureDecision(testFeatureKey, testUserContext)
	assert.Error(t, err)
	assert.EqualError(t, err, "I'm panicking")
}

func TestGetFeatureDecisionErrFeatureDecision(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "teststring"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "defaultString",
		ID:           "1",
		Key:          testVariableKey,
		Type:         entities.String,
	}
	testVariation := getTestVariationWithFeatureVariable(false, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, errors.New("error feature"))

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}

	_, decision, err := client.getFeatureDecision(testFeatureKey, testUserContext)
	assert.Equal(t, expectedFeatureDecision, decision)
	assert.NoError(t, err)
}

func TestGetAllFeatureVariables(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "teststring"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "defaultString",
		ID:           "1",
		Key:          testVariableKey,
		Type:         entities.String,
	}
	testVariation := getTestVariationWithFeatureVariable(false, testVariationVariable)
	testVariation.FeatureEnabled = true
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	testFeature.VariableMap = map[string]entities.Variable{testVariable.Key: testVariable}
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}

	enabled, variationMap, err := client.GetAllFeatureVariables(testFeatureKey, testUserContext)
	assert.True(t, enabled)
	assert.Equal(t, testVariableValue, variationMap[testVariableKey])
	assert.Nil(t, err)
}

func TestGetAllFeatureVariablesWithError(t *testing.T) {
	testFeatureKey := "test_feature_key"
	testVariableKey := "test_feature_flag_key"
	testVariableValue := "teststring"
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationVariable := entities.VariationVariable{
		ID:    "1",
		Value: testVariableValue,
	}
	testVariable := entities.Variable{
		DefaultValue: "defaultString",
		ID:           "1",
		Key:          testVariableKey,
		Type:         entities.String,
	}
	testVariation := getTestVariationWithFeatureVariable(true, testVariationVariable)
	testExperiment := entities.Experiment{
		ID:         "111111",
		Variations: map[string]entities.Variation{"22222": testVariation},
	}
	testFeature := getTestFeature(testFeatureKey, testExperiment)
	testFeature.VariableMap = map[string]entities.Variable{testVariable.Key: testVariable}
	mockConfig := getMockConfig(testFeatureKey, testVariableKey, testFeature, testVariable)
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(mockConfig, nil)

	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: mockConfig,
	}

	expectedFeatureDecision := getTestFeatureDecision(testExperiment, testVariation, true)
	mockDecisionService := new(MockDecisionService)
	mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, errors.New(""))

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: mockDecisionService,
	}

	enabled, variationMap, err := client.GetAllFeatureVariables(testFeatureKey, testUserContext)

	// if we have a decision, but also a non-fatal error, we should return the decision
	assert.True(t, enabled)
	assert.Equal(t, testVariableValue, variationMap[testVariableKey])
	assert.NoError(t, err)
}

// Helper Methods
func getTestFeatureDecision(experiment entities.Experiment, variation entities.Variation, decisionMade bool) decision.FeatureDecision {
	return decision.FeatureDecision{
		Experiment: experiment,
		Variation:  &variation,
	}
}

func getTestVariationWithFeatureVariable(featureEnabled bool, variable entities.VariationVariable) entities.Variation {
	return entities.Variation{
		ID:             "22222",
		Key:            "22222",
		FeatureEnabled: featureEnabled,
		Variables:      map[string]entities.VariationVariable{variable.ID: variable},
	}
}

func getMockConfig(featureKey string, variableKey string, feature entities.Feature, variable entities.Variable) *MockProjectConfig {
	mockConfig := new(MockProjectConfig)
	mockConfig.On("GetFeatureByKey", featureKey).Return(feature, nil)
	mockConfig.On("GetVariableByKey", featureKey, variableKey).Return(variable, nil)
	return mockConfig
}

func getTestFeature(featureKey string, experiment entities.Experiment) entities.Feature {
	return entities.Feature{
		ID:                 "22222",
		Key:                featureKey,
		FeatureExperiments: []entities.Experiment{experiment},
	}
}

type ClientTestSuiteAB struct {
	suite.Suite
	mockConfig          *MockProjectConfig
	mockConfigManager   *MockProjectConfigManager
	mockDecisionService *MockDecisionService
	mockEventProcessor  *MockEventProcessor
}

func (s *ClientTestSuiteAB) SetupTest() {
	s.mockConfig = new(MockProjectConfig)
	s.mockConfigManager = new(MockProjectConfigManager)
	s.mockConfigManager.On("GetConfig").Return(s.mockConfig, nil)
	s.mockDecisionService = new(MockDecisionService)
	s.mockEventProcessor = new(MockEventProcessor)
}

func (s *ClientTestSuiteAB) TestActivate() {
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testExperiment := makeTestExperiment("test_exp_1")
	s.mockConfig.On("GetExperimentByKey", "test_exp_1").Return(testExperiment, nil)
	s.mockConfig.On("GetExperimentByKey", "test_exp_2").Return(testExperiment, errors.New("Experiment not found"))

	testDecisionContext := decision.ExperimentDecisionContext{
		Experiment:    &testExperiment,
		ProjectConfig: s.mockConfig,
	}

	expectedVariation := testExperiment.Variations["v2"]
	expectedExperimentDecision := decision.ExperimentDecision{
		Variation: &expectedVariation,
	}
	s.mockDecisionService.On("GetExperimentDecision", testDecisionContext, testUserContext).Return(expectedExperimentDecision, nil)
	s.mockEventProcessor.On("ProcessEvent", mock.AnythingOfType("event.UserEvent"))

	testClient := OptimizelyClient{
		ConfigManager:   s.mockConfigManager,
		DecisionService: s.mockDecisionService,
		EventProcessor:  s.mockEventProcessor,
	}

	variationKey1, err1 := testClient.Activate("test_exp_1", testUserContext)
	s.NoError(err1)
	s.Equal(expectedVariation.Key, variationKey1)

	// should not return error for experiment not found.
	variationKey2, err2 := testClient.Activate("test_exp_2", testUserContext)
	s.NoError(err2)
	s.Equal("", variationKey2)

	s.mockConfig.AssertExpectations(s.T())
	s.mockDecisionService.AssertExpectations(s.T())
	s.mockEventProcessor.AssertExpectations(s.T())
}

func (s *ClientTestSuiteAB) TestActivatePanics() {
	// ensure that we recover if the SDK panics while getting variation
	testUserContext := entities.UserContext{}
	testClient := OptimizelyClient{
		ConfigManager:   new(PanickingConfigManager),
		DecisionService: s.mockDecisionService,
	}

	variationKey, err := testClient.Activate("test_exp_1", testUserContext)
	s.Equal("", variationKey)
	s.EqualError(err, "I'm panicking")
}

func (s *ClientTestSuiteAB) TestActivateInvalidConfig() {
	testUserContext := entities.UserContext{}

	mockConfigManager := new(MockProjectConfigManager)
	expectedError := errors.New("no project config available")
	mockConfigManager.On("GetConfig").Return(s.mockConfig, expectedError)
	testClient := OptimizelyClient{
		ConfigManager: mockConfigManager,
	}

	variationKey, err := testClient.Activate("test_exp_1", testUserContext)
	s.Equal("", variationKey)
	s.Error(err)
	s.Equal(expectedError, err)
}

func (s *ClientTestSuiteAB) TestGetVariation() {
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testExperiment := makeTestExperiment("test_exp_1")
	s.mockConfig.On("GetExperimentByKey", "test_exp_1").Return(testExperiment, nil)

	testDecisionContext := decision.ExperimentDecisionContext{
		Experiment:    &testExperiment,
		ProjectConfig: s.mockConfig,
	}

	expectedVariation := testExperiment.Variations["v2"]
	expectedExperimentDecision := decision.ExperimentDecision{
		Variation: &expectedVariation,
	}
	s.mockDecisionService.On("GetExperimentDecision", testDecisionContext, testUserContext).Return(expectedExperimentDecision, nil)

	testClient := OptimizelyClient{
		ConfigManager:   s.mockConfigManager,
		DecisionService: s.mockDecisionService,
	}

	variationKey, err := testClient.GetVariation("test_exp_1", testUserContext)
	s.NoError(err)
	s.Equal(expectedVariation.Key, variationKey)
	s.mockConfig.AssertExpectations(s.T())
	s.mockDecisionService.AssertExpectations(s.T())
	s.mockEventProcessor.AssertNotCalled(s.T(), "ProcessEvent", mock.AnythingOfType("event.UserEvent"))
}

func (s *ClientTestSuiteAB) TestGetVariationWithDecisionError() {
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testExperiment := makeTestExperiment("test_exp_1")
	s.mockConfig.On("GetExperimentByKey", "test_exp_1").Return(testExperiment, nil)

	testDecisionContext := decision.ExperimentDecisionContext{
		Experiment:    &testExperiment,
		ProjectConfig: s.mockConfig,
	}

	expectedVariation := testExperiment.Variations["v2"]
	expectedExperimentDecision := decision.ExperimentDecision{
		Variation: &expectedVariation,
	}
	s.mockDecisionService.On("GetExperimentDecision", testDecisionContext, testUserContext).Return(expectedExperimentDecision, errors.New(""))

	testClient := OptimizelyClient{
		ConfigManager:   s.mockConfigManager,
		DecisionService: s.mockDecisionService,
	}

	variationKey, err := testClient.GetVariation("test_exp_1", testUserContext)
	s.NoError(err)
	s.Equal(expectedVariation.Key, variationKey)
	s.mockConfig.AssertExpectations(s.T())
	s.mockDecisionService.AssertExpectations(s.T())
	s.mockEventProcessor.AssertNotCalled(s.T(), "ProcessEvent", mock.AnythingOfType("event.UserEvent"))
}

func (s *ClientTestSuiteAB) TestGetVariationPanics() {
	// ensure that we recover if the SDK panics while getting variation
	testUserContext := entities.UserContext{}
	testClient := OptimizelyClient{
		ConfigManager:   new(PanickingConfigManager),
		DecisionService: s.mockDecisionService,
	}

	variationKey, err := testClient.GetVariation("test_exp_1", testUserContext)
	s.Equal("", variationKey)
	s.EqualError(err, "I'm panicking")
}

type ClientTestSuiteFM struct {
	suite.Suite
	mockConfig          *MockProjectConfig
	mockConfigManager   *MockProjectConfigManager
	mockDecisionService *MockDecisionService
	mockEventProcessor  *MockEventProcessor
}

func (s *ClientTestSuiteFM) SetupTest() {
	s.mockConfig = new(MockProjectConfig)
	s.mockConfigManager = new(MockProjectConfigManager)
	s.mockConfigManager.On("GetConfig").Return(s.mockConfig, nil)
	s.mockDecisionService = new(MockDecisionService)
	s.mockEventProcessor = new(MockEventProcessor)
}

func (s *ClientTestSuiteFM) TestIsFeatureEnabled() {
	testUserContext := entities.UserContext{ID: "test_user_1"}

	// Test happy path
	testVariation := makeTestVariation("green", true)
	testExperiment := makeTestExperimentWithVariations("number_1", []entities.Variation{testVariation})
	testFeature := makeTestFeatureWithExperiment("feature_1", testExperiment)
	s.mockConfig.On("GetFeatureByKey", testFeature.Key).Return(testFeature, nil)
	s.mockConfigManager.On("GetConfig").Return(s.mockConfig, nil)

	// Set up the mock decision service and its return value
	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: s.mockConfig,
	}

	expectedFeatureDecision := decision.FeatureDecision{
		Experiment: testExperiment,
		Variation:  &testVariation,
		Source:     decision.FeatureTest,
	}

	s.mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, nil)

	client := OptimizelyClient{
		ConfigManager:   s.mockConfigManager,
		DecisionService: s.mockDecisionService,
	}
	result, _ := client.IsFeatureEnabled(testFeature.Key, testUserContext)
	s.True(result)
	s.mockConfig.AssertExpectations(s.T())
	s.mockConfigManager.AssertExpectations(s.T())
	s.mockDecisionService.AssertExpectations(s.T())
}

func (s *ClientTestSuiteFM) TestIsFeatureEnabledWithDecisionError() {
	testUserContext := entities.UserContext{ID: "test_user_1"}

	// Test happy path
	testVariation := makeTestVariation("green", true)
	testExperiment := makeTestExperimentWithVariations("number_1", []entities.Variation{testVariation})
	testFeature := makeTestFeatureWithExperiment("feature_1", testExperiment)
	s.mockConfig.On("GetFeatureByKey", testFeature.Key).Return(testFeature, nil)
	s.mockConfigManager.On("GetConfig").Return(s.mockConfig, nil)

	// Set up the mock decision service and its return value
	testDecisionContext := decision.FeatureDecisionContext{
		Feature:       &testFeature,
		ProjectConfig: s.mockConfig,
	}

	expectedFeatureDecision := decision.FeatureDecision{
		Experiment: testExperiment,
		Variation:  &testVariation,
		Source:     decision.FeatureTest,
	}

	s.mockDecisionService.On("GetFeatureDecision", testDecisionContext, testUserContext).Return(expectedFeatureDecision, errors.New(""))
	s.mockEventProcessor.On("ProcessEvent", mock.AnythingOfType("event.UserEvent"))

	client := OptimizelyClient{
		ConfigManager:   s.mockConfigManager,
		DecisionService: s.mockDecisionService,
		EventProcessor:  s.mockEventProcessor,
	}

	// should still return the decision because the error is non-fatal
	result, err := client.IsFeatureEnabled(testFeature.Key, testUserContext)
	s.True(result)
	s.NoError(err)
	s.mockConfig.AssertExpectations(s.T())
	s.mockConfigManager.AssertExpectations(s.T())
	s.mockDecisionService.AssertExpectations(s.T())
}

func (s *ClientTestSuiteFM) TestIsFeatureEnabledErrorCases() {
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testFeatureKey := "test_feature_key"

	// Test instance invalid
	s.mockConfigManager.On("GetConfig").Return(nil, errors.New("no project config available"))

	client := OptimizelyClient{
		ConfigManager:   s.mockConfigManager,
		DecisionService: s.mockDecisionService,
	}
	result, _ := client.IsFeatureEnabled(testFeatureKey, testUserContext)
	s.False(result)
	s.mockDecisionService.AssertNotCalled(s.T(), "GetFeatureDecision")

	// Test invalid feature key
	expectedError := errors.New("Invalid feature key")
	s.mockConfig.On("GetFeatureByKey", testFeatureKey).Return(entities.Feature{}, expectedError)
	s.mockConfigManager.On("GetConfig").Return(s.mockConfig, nil)

	client = OptimizelyClient{
		ConfigManager:   s.mockConfigManager,
		DecisionService: s.mockDecisionService,
	}
	result, err := client.IsFeatureEnabled(testFeatureKey, testUserContext)
	s.NoError(err)
	s.False(result)
	s.mockConfigManager.AssertExpectations(s.T())
	s.mockDecisionService.AssertNotCalled(s.T(), "GetDecision")
}

func (s *ClientTestSuiteFM) TestIsFeatureEnabledPanic() {
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testFeatureKey := "test_feature_key"

	client := OptimizelyClient{
		ConfigManager: &PanickingConfigManager{},
	}

	// ensure that the client calms back down and recovers
	result, err := client.IsFeatureEnabled(testFeatureKey, testUserContext)
	s.False(result)
	s.Error(err)
}

func (s *ClientTestSuiteFM) TestGetEnabledFeatures() {
	testUserContext := entities.UserContext{ID: "test_user_1"}
	testVariationEnabled := makeTestVariation("a", true)
	testVariationDisabled := makeTestVariation("b", false)
	testExperimentEnabled := makeTestExperimentWithVariations("enabled_exp", []entities.Variation{testVariationEnabled})
	testExperimentDisabled := makeTestExperimentWithVariations("disabled_exp", []entities.Variation{testVariationDisabled})
	testFeatureEnabled := makeTestFeatureWithExperiment("enabled_feat", testExperimentEnabled)
	testFeatureDisabled := makeTestFeatureWithExperiment("disabled_feat", testExperimentDisabled)

	featureList := []entities.Feature{testFeatureEnabled, testFeatureDisabled}
	s.mockConfig.On("GetFeatureByKey", testFeatureEnabled.Key).Return(testFeatureEnabled, nil)
	s.mockConfig.On("GetFeatureByKey", testFeatureDisabled.Key).Return(testFeatureDisabled, nil)
	s.mockConfig.On("GetFeatureList").Return(featureList)
	s.mockConfigManager.On("GetConfig").Return(s.mockConfig, nil)

	testDecisionContextEnabled := decision.FeatureDecisionContext{
		Feature:       &testFeatureEnabled,
		ProjectConfig: s.mockConfig,
	}
	testDecisionContextDisabled := decision.FeatureDecisionContext{
		Feature:       &testFeatureDisabled,
		ProjectConfig: s.mockConfig,
	}

	expectedFeatureDecisionEnabled := decision.FeatureDecision{
		Experiment: testExperimentEnabled,
		Variation:  &testVariationEnabled,
	}
	expectedFeatureDecisionDisabled := decision.FeatureDecision{
		Experiment: testExperimentDisabled,
		Variation:  &testVariationDisabled,
	}

	s.mockDecisionService.On("GetFeatureDecision", testDecisionContextEnabled, testUserContext).Return(expectedFeatureDecisionEnabled, nil)
	s.mockDecisionService.On("GetFeatureDecision", testDecisionContextDisabled, testUserContext).Return(expectedFeatureDecisionDisabled, nil)

	client := OptimizelyClient{
		ConfigManager:   s.mockConfigManager,
		DecisionService: s.mockDecisionService,
	}
	result, err := client.GetEnabledFeatures(testUserContext)
	s.NoError(err)
	s.ElementsMatch(result, []string{testFeatureEnabled.Key})
	s.mockConfig.AssertExpectations(s.T())
	s.mockConfigManager.AssertExpectations(s.T())
	s.mockDecisionService.AssertExpectations(s.T())
}

func (s *ClientTestSuiteFM) TestGetEnabledFeaturesErrorCases() {
	testUserContext := entities.UserContext{ID: "test_user_1"}

	// Test instance invalid
	expectedError := errors.New("no project config available")
	mockConfigManager := new(MockProjectConfigManager)
	mockConfigManager.On("GetConfig").Return(s.mockConfig, expectedError)

	client := OptimizelyClient{
		ConfigManager:   mockConfigManager,
		DecisionService: s.mockDecisionService,
	}
	result, err := client.GetEnabledFeatures(testUserContext)
	s.Error(err)
	s.Equal(expectedError, err)
	s.Empty(result)
	mockConfigManager.AssertNotCalled(s.T(), "GetFeatureByKey")
	s.mockDecisionService.AssertNotCalled(s.T(), "GetFeatureDecision")
}

func TestClose(t *testing.T) {
	mockProcessor := &MockProcessor{}
	mockDecisionService := new(MockDecisionService)

	client := OptimizelyClient{
		ConfigManager:   ValidProjectConfigManager(),
		DecisionService: mockDecisionService,
		EventProcessor:  mockProcessor,
		executionCtx:    new(ExecutionCtx),
	}

	assert.False(t, exeCtxSignalFlag)
	client.Close()
	assert.True(t, exeCtxSignalFlag)

}

func TestClientTestSuite(t *testing.T) {
	suite.Run(t, new(ClientTestSuiteAB))
	suite.Run(t, new(ClientTestSuiteFM))
}