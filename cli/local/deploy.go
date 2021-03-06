/*
Copyright 2020 Cortex Labs, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package local

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cortexlabs/cortex/cli/types/cliconfig"
	"github.com/cortexlabs/cortex/pkg/consts"
	"github.com/cortexlabs/cortex/pkg/lib/aws"
	"github.com/cortexlabs/cortex/pkg/lib/docker"
	"github.com/cortexlabs/cortex/pkg/lib/errors"
	"github.com/cortexlabs/cortex/pkg/lib/files"
	"github.com/cortexlabs/cortex/pkg/lib/gcp"
	"github.com/cortexlabs/cortex/pkg/operator/schema"
	"github.com/cortexlabs/cortex/pkg/types"
	"github.com/cortexlabs/cortex/pkg/types/spec"
	"github.com/cortexlabs/cortex/pkg/types/userconfig"
)

func Deploy(env cliconfig.Environment, configPath string, projectFileList []string, disallowPrompt bool) ([]schema.DeployResult, error) {
	configFileName := filepath.Base(configPath)

	_, err := docker.GetDockerClient()
	if err != nil {
		return nil, err
	}

	configBytes, err := files.ReadFileBytes(configPath)
	if err != nil {
		return nil, err
	}

	if !files.IsAbsOrTildePrefixed(configPath) {
		return nil, errors.ErrorUnexpected(fmt.Sprintf("%s is not an absolute path", configPath))
	}
	projectRoot := files.Dir(configPath)

	projectFiles, err := newProjectFiles(projectFileList, projectRoot)
	if err != nil {
		return nil, err
	}

	apiConfigs, err := spec.ExtractAPIConfigs(configBytes, types.LocalProviderType, configFileName, nil, nil)
	if err != nil {
		return nil, err
	}

	return deploy(env, apiConfigs, projectFiles, disallowPrompt)
}

func deploy(env cliconfig.Environment, apiConfigs []userconfig.API, projectFiles ProjectFiles, disallowPrompt bool) ([]schema.DeployResult, error) {
	var err error
	var awsClient *aws.Client
	var gcpClient *gcp.Client

	if env.AWSAccessKeyID != nil {
		awsClient, err = aws.NewFromCreds(*env.AWSRegion, *env.AWSAccessKeyID, *env.AWSSecretAccessKey)
		if err != nil {
			return nil, err
		}
	} else {
		awsClient, err = aws.NewAnonymousClient()
		if err != nil {
			return nil, err
		}
	}

	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" {
		gcpClient, err = gcp.NewFromEnv()
		if err != nil {
			return nil, err
		}
	}

	if awsClient == nil && hasAnyModelWithPrefix(apiConfigs, "s3://") {
		return nil, ErrorMustSpecifyLocalAWSCreds()
	}

	if gcpClient == nil && hasAnyModelWithPrefix(apiConfigs, "gs://") {
		return nil, gcp.ErrorCredentialsFileEnvVarNotSet()
	}

	models := []spec.CuratedModelResource{}
	err = ValidateLocalAPIs(apiConfigs, &models, projectFiles, awsClient, gcpClient)
	if err != nil {
		err = errors.Append(err, fmt.Sprintf("\n\napi configuration schema for Realtime API can be found at https://docs.cortex.dev/v/%s/deployments/realtime-api/api-configuration", consts.CortexVersionMinor))
		return nil, err
	}

	projectRelFilePaths := projectFiles.AllAbsPaths()
	projectID, err := files.HashFile(projectRelFilePaths[0], projectRelFilePaths[1:]...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to hash directory", projectFiles.projectRoot)
	}

	results := make([]schema.DeployResult, len(apiConfigs))
	for i := range apiConfigs {
		apiConfig := apiConfigs[i]
		api, msg, err := UpdateAPI(&apiConfig, models, projectFiles.projectRoot, projectID, disallowPrompt, awsClient, gcpClient)
		results[i].Message = msg
		if err != nil {
			results[i].Error = errors.Message(err)
		} else {
			results[i].API = api
		}
	}

	return results, nil
}

func hasAnyModelWithPrefix(apiConfigs []userconfig.API, modelPrefix string) bool {
	for _, apiConfig := range apiConfigs {
		if apiConfig.Predictor.ModelPath != nil && strings.HasPrefix(*apiConfig.Predictor.ModelPath, modelPrefix) {
			return true
		}
		if apiConfig.Predictor.Models != nil {
			if apiConfig.Predictor.Models.Dir != nil && strings.HasPrefix(*apiConfig.Predictor.ModelPath, modelPrefix) {
				return true
			}
			for _, model := range apiConfig.Predictor.Models.Paths {
				if model == nil {
					continue
				}
				if strings.HasPrefix(model.ModelPath, modelPrefix) {
					return true
				}
			}
		}
	}

	return false
}
