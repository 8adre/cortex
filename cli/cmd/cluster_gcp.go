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

package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cortexlabs/cortex/cli/cluster"
	"github.com/cortexlabs/cortex/cli/types/cliconfig"
	"github.com/cortexlabs/cortex/pkg/lib/console"
	"github.com/cortexlabs/cortex/pkg/lib/docker"
	"github.com/cortexlabs/cortex/pkg/lib/errors"
	"github.com/cortexlabs/cortex/pkg/lib/exit"
	"github.com/cortexlabs/cortex/pkg/lib/gcp"
	"github.com/cortexlabs/cortex/pkg/lib/k8s"
	"github.com/cortexlabs/cortex/pkg/lib/pointer"
	"github.com/cortexlabs/cortex/pkg/lib/prompt"
	s "github.com/cortexlabs/cortex/pkg/lib/strings"
	"github.com/cortexlabs/cortex/pkg/lib/telemetry"
	"github.com/cortexlabs/cortex/pkg/types"
	"github.com/cortexlabs/cortex/pkg/types/clusterconfig"
	"github.com/spf13/cobra"
	containerpb "google.golang.org/genproto/googleapis/container/v1"
)

var (
	_flagClusterGCPUpEnv          string
	_flagClusterGCPInfoEnv        string
	_flagClusterGCPInfoDebug      bool
	_flagClusterGCPConfig         string
	_flagClusterGCPName           string
	_flagClusterGCPZone           string
	_flagClusterGCPProject        string
	_flagClusterGCPDisallowPrompt bool
)

func clusterGCPInit() {
	_clusterGCPUpCmd.Flags().SortFlags = false
	addClusterGCPConfigFlag(_clusterGCPUpCmd)
	defaultEnv := getDefaultEnv(_clusterGCPCommandType)
	_clusterGCPUpCmd.Flags().StringVarP(&_flagClusterGCPUpEnv, "configure-env", "e", defaultEnv, "name of environment to configure")
	addClusterGCPDisallowPromptFlag(_clusterGCPUpCmd)
	_clusterGCPCmd.AddCommand(_clusterGCPUpCmd)

	_clusterGCPInfoCmd.Flags().SortFlags = false
	addClusterGCPConfigFlag(_clusterGCPInfoCmd)
	addClusterGCPNameFlag(_clusterGCPInfoCmd)
	addClusterGCPProjectFlag(_clusterGCPInfoCmd)
	addClusterGCPZoneFlag(_clusterGCPInfoCmd)
	_clusterGCPInfoCmd.Flags().StringVarP(&_flagClusterGCPInfoEnv, "configure-env", "e", "", "name of environment to configure")
	_clusterGCPInfoCmd.Flags().BoolVarP(&_flagClusterGCPInfoDebug, "debug", "d", false, "save the current cluster state to a file")
	_clusterGCPInfoCmd.Flags().BoolVarP(&_flagClusterGCPDisallowPrompt, "yes", "y", false, "skip prompts")
	_clusterGCPCmd.AddCommand(_clusterGCPInfoCmd)

	_clusterGCPDownCmd.Flags().SortFlags = false
	addClusterGCPConfigFlag(_clusterGCPDownCmd)
	addClusterGCPNameFlag(_clusterGCPDownCmd)
	addClusterGCPProjectFlag(_clusterGCPDownCmd)
	addClusterGCPZoneFlag(_clusterGCPDownCmd)
	addClusterGCPDisallowPromptFlag(_clusterGCPDownCmd)
	_clusterGCPCmd.AddCommand(_clusterGCPDownCmd)
}

func addClusterGCPConfigFlag(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&_flagClusterGCPConfig, "config", "c", "", "path to a cluster configuration file")
	cmd.Flags().SetAnnotation("config", cobra.BashCompFilenameExt, _configFileExts)
}

func addClusterGCPNameFlag(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&_flagClusterGCPName, "name", "n", "", "name of the cluster")
}

func addClusterGCPZoneFlag(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&_flagClusterGCPZone, "zone", "z", "", "gcp zone of the cluster")
}

func addClusterGCPProjectFlag(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&_flagClusterGCPProject, "project", "p", "", "gcp project id")
}

func addClusterGCPDisallowPromptFlag(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&_flagClusterGCPDisallowPrompt, "yes", "y", false, "skip prompts")
}

var _clusterGCPCmd = &cobra.Command{
	Use:   "cluster-gcp",
	Short: "manage GCP clusters (contains subcommands)",
}

var _clusterGCPUpCmd = &cobra.Command{
	Use:   "up",
	Short: "spin up a cluster on gcp",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		telemetry.EventNotify("cli.cluster.up", map[string]interface{}{"provider": types.GCPProviderType})

		if _flagClusterGCPUpEnv == "local" {
			exit.Error(ErrorLocalEnvironmentCantUseClusterProvider(types.GCPProviderType))
		}

		envExists, err := isEnvConfigured(_flagClusterGCPUpEnv)
		if err != nil {
			exit.Error(err)
		}
		if envExists {
			if _flagClusterGCPDisallowPrompt {
				fmt.Printf("found an existing environment named \"%s\", which will be overwritten to connect to this cluster once it's created\n\n", _flagClusterGCPUpEnv)
			} else {
				prompt.YesOrExit(fmt.Sprintf("found an existing environment named \"%s\"; would you like to overwrite it to connect to this cluster once it's created?", _flagClusterGCPUpEnv), "", "you can specify a different environment name to be configured to connect to this cluster by specifying the --configure-env flag (e.g. `cortex cluster up --configure-env prod`); or you can list your environments with `cortex env list` and delete an environment with `cortex env delete ENV_NAME`")
			}
		}

		if _, err := docker.GetDockerClient(); err != nil {
			exit.Error(err)
		}

		if !_flagClusterGCPDisallowPrompt {
			promptForEmail()
		}

		accessConfig, err := getNewGCPClusterAccessConfig(_flagClusterGCPDisallowPrompt)
		if err != nil {
			exit.Error(err)
		}

		gcpClient, err := gcp.NewFromEnvCheckProjectID(*accessConfig.Project)
		if err != nil {
			exit.Error(err)
		}

		clusterConfig, err := getGCPInstallClusterConfig(gcpClient, *accessConfig, _flagClusterGCPDisallowPrompt)
		if err != nil {
			exit.Error(err)
		}

		bucketName := clusterconfig.GCPBucketName(*accessConfig.ClusterName, *accessConfig.Project, *accessConfig.Zone)
		err = gcpClient.CreateBucket(bucketName, gcp.ZoneToRegion(*accessConfig.Zone), true)
		if err != nil {
			exit.Error(err)
		}

		err = createGKECluster(clusterConfig, gcpClient)
		if err != nil {
			gcpClient.DeleteBucket(bucketName)
			exit.Error(err)
		}

		_, _, err = runGCPManagerWithClusterConfig("/root/install.sh", clusterConfig, bucketName, nil, nil)
		if err != nil {
			gcpClient.DeleteBucket(bucketName)
			exit.Error(err)
		}

		gkeClusterName := fmt.Sprintf("projects/%s/locations/%s/clusters/%s", *clusterConfig.Project, *clusterConfig.Zone, clusterConfig.ClusterName)
		operatorLoadBalancerIP, err := getGCPOperatorLoadBalancerIP(gkeClusterName, gcpClient)
		if err != nil {
			exit.Error(errors.Append(err, fmt.Sprintf("\n\nyou can attempt to resolve this issue and configure your cli environment by running `cortex cluster info --configure-env %s`", _flagClusterGCPUpEnv)))
		}

		newEnvironment := cliconfig.Environment{
			Name:             _flagClusterGCPUpEnv,
			Provider:         types.GCPProviderType,
			OperatorEndpoint: &operatorLoadBalancerIP,
		}

		err = addEnvToCLIConfig(newEnvironment)
		if err != nil {
			exit.Error(errors.Append(err, fmt.Sprintf("\n\nyou can attempt to resolve this issue and configure your cli environment by running `cortex cluster info --configure-env %s`", _flagClusterGCPUpEnv)))
		}

		if envExists {
			fmt.Printf(console.Bold("\nthe environment named \"%s\" has been updated to point to this cluster; append `--env %s` to cortex commands to use this cluster (e.g. `cortex deploy --env %s`), or set it as your default with `cortex env default %s`\n"), _flagClusterGCPUpEnv, _flagClusterGCPUpEnv, _flagClusterGCPUpEnv, _flagClusterGCPUpEnv)
		} else {
			fmt.Printf(console.Bold("\nan environment named \"%s\" has been configured to point to this cluster; append `--env %s` to cortex commands to use this cluster (e.g. `cortex deploy --env %s`), or set it as your default with `cortex env default %s`\n"), _flagClusterGCPUpEnv, _flagClusterGCPUpEnv, _flagClusterGCPUpEnv, _flagClusterGCPUpEnv)
		}
	},
}

var _clusterGCPInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "get information about a cluster",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		telemetry.Event("cli.cluster.info", map[string]interface{}{"provider": types.GCPProviderType})

		if _flagClusterGCPInfoEnv == "local" {
			exit.Error(ErrorLocalEnvironmentCantUseClusterProvider(types.GCPProviderType))
		}

		if _, err := docker.GetDockerClient(); err != nil {
			exit.Error(err)
		}

		accessConfig, err := getGCPClusterAccessConfigWithCache(_flagClusterGCPDisallowPrompt)
		if err != nil {
			exit.Error(err)
		}

		if _flagClusterGCPInfoDebug {
			cmdDebugGCP(accessConfig)
		} else {
			cmdInfoGCP(accessConfig, _flagClusterGCPDisallowPrompt)
		}
	},
}

var _clusterGCPDownCmd = &cobra.Command{
	Use:   "down",
	Short: "spin down a cluster",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		telemetry.Event("cli.cluster.down", map[string]interface{}{"provider": types.GCPProviderType})

		if _flagClusterGCPUpEnv == "local" {
			exit.Error(ErrorLocalEnvironmentCantUseClusterProvider(types.GCPProviderType))
		}

		if _, err := docker.GetDockerClient(); err != nil {
			exit.Error(err)
		}

		accessConfig, err := getGCPClusterAccessConfigWithCache(_flagClusterGCPDisallowPrompt)
		if err != nil {
			exit.Error(err)
		}

		gcpClient, err := gcp.NewFromEnvCheckProjectID(*accessConfig.Project)
		if err != nil {
			exit.Error(err)
		}

		gkeClusterName := fmt.Sprintf("projects/%s/locations/%s/clusters/%s", *accessConfig.Project, *accessConfig.Zone, *accessConfig.ClusterName)

		// updating CLI env is best-effort, so ignore errors
		operatorLoadBalancerIP, _ := getGCPOperatorLoadBalancerIP(gkeClusterName, gcpClient)

		if _flagClusterGCPDisallowPrompt {
			fmt.Printf("your cluster named \"%s\" in %s (zone: %s) will be spun down and all apis will be deleted\n\n", *accessConfig.ClusterName, *accessConfig.Project, *accessConfig.Zone)
		} else {
			prompt.YesOrExit(fmt.Sprintf("your cluster named \"%s\" in %s (zone: %s) will be spun down and all apis will be deleted, are you sure you want to continue?", *accessConfig.ClusterName, *accessConfig.Project, *accessConfig.Zone), "", "")
		}

		bucketName := clusterconfig.GCPBucketName(*accessConfig.ClusterName, *accessConfig.Project, *accessConfig.Zone)
		fmt.Printf("￮ deleting bucket %s ", bucketName)
		err = gcpClient.DeleteBucket(bucketName)
		if err != nil {
			fmt.Printf("\n\nunable to delete cortex's bucket (see error below); if it still exists after the cluster has been deleted, please delete it via the the GCP console\n")
			errors.PrintError(err)
			fmt.Println()
		} else {
			fmt.Println("✓")
		}

		fmt.Print("￮ spinning down the cluster ")

		_, err = gcpClient.DeleteCluster(gkeClusterName)
		if err != nil {
			fmt.Print("\n\n")
			exit.Error(err)
		}

		fmt.Println("✓")

		// best-effort deletion of cli environment(s)
		if operatorLoadBalancerIP != "" {
			envNames, isDefaultEnv, _ := getEnvNamesByOperatorEndpoint(operatorLoadBalancerIP)
			if len(envNames) > 0 {
				for _, envName := range envNames {
					removeEnvFromCLIConfig(envName)
				}
				fmt.Printf("✓ deleted the %s environment configuration%s\n", s.StrsAnd(envNames), s.SIfPlural(len(envNames)))
				if isDefaultEnv {
					fmt.Println("✓ set the default environment to local")
				}
			}
		}

		cachedClusterConfigPath := cachedGCPClusterConfigPath(*accessConfig.ClusterName, *accessConfig.Project, *accessConfig.Zone)
		os.Remove(cachedClusterConfigPath)
	},
}

func cmdInfoGCP(accessConfig *clusterconfig.GCPAccessConfig, disallowPrompt bool) {
	fmt.Print("fetching cluster endpoints ...\n\n")
	out, exitCode, err := runGCPManagerAccessCommand("/root/info_gcp.sh", *accessConfig, nil, nil)
	if err != nil {
		exit.Error(err)
	}
	if exitCode == nil || *exitCode != 0 {
		exit.Error(ErrorClusterInfo(out))
	}

	fmt.Println()

	var operatorEndpoint string
	for _, line := range strings.Split(out, "\n") {
		// before modifying this, search for this prefix
		if strings.HasPrefix(line, "operator: ") {
			operatorEndpoint = "https://" + strings.TrimSpace(strings.TrimPrefix(line, "operator: "))
			break
		}
	}

	if err := printInfoOperatorResponseGCP(accessConfig, operatorEndpoint); err != nil {
		exit.Error(err)
	}

	if _flagClusterGCPInfoEnv != "" {
		if err := updateGCPCLIEnv(_flagClusterGCPInfoEnv, operatorEndpoint, disallowPrompt); err != nil {
			exit.Error(err)
		}
	}
}

func printInfoOperatorResponseGCP(accessConfig *clusterconfig.GCPAccessConfig, operatorEndpoint string) error {
	fmt.Print("fetching cluster status ...\n\n")

	operatorConfig := cluster.OperatorConfig{
		Telemetry:        isTelemetryEnabled(),
		ClientID:         clientID(),
		OperatorEndpoint: operatorEndpoint,
	}

	infoResponse, err := cluster.InfoGCP(operatorConfig)
	if err != nil {
		return err
	}

	infoResponse.ClusterConfig.UserTable().Print()

	return nil
}

func cmdDebugGCP(accessConfig *clusterconfig.GCPAccessConfig) {
	// note: if modifying this string, also change it in files.IgnoreCortexDebug()
	debugFileName := fmt.Sprintf("cortex-debug-%s.tgz", time.Now().UTC().Format("2006-01-02-15-04-05"))

	containerDebugPath := "/out/" + debugFileName
	copyFromPaths := []dockerCopyFromPath{
		{
			containerPath: containerDebugPath,
			localDir:      _cwd,
		},
	}

	out, exitCode, err := runGCPManagerAccessCommand("/root/debug_gcp.sh "+containerDebugPath, *accessConfig, nil, copyFromPaths)
	if err != nil {
		exit.Error(err)
	}
	if exitCode == nil || *exitCode != 0 {
		exit.Error(ErrorClusterDebug(out))
	}

	fmt.Println("saved cluster info to ./" + debugFileName)
	return
}

func updateGCPCLIEnv(envName string, operatorEndpoint string, disallowPrompt bool) error {
	prevEnv, err := readEnv(envName)
	if err != nil {
		return err
	}

	newEnvironment := cliconfig.Environment{
		Name:             envName,
		Provider:         types.GCPProviderType,
		OperatorEndpoint: pointer.String(operatorEndpoint),
	}

	shouldWriteEnv := false
	envWasUpdated := false
	if prevEnv == nil {
		shouldWriteEnv = true
		fmt.Println()
	} else if *prevEnv.OperatorEndpoint != operatorEndpoint {
		envWasUpdated = true
		if disallowPrompt {
			shouldWriteEnv = true
			fmt.Println()
		} else {
			shouldWriteEnv = prompt.YesOrNo(fmt.Sprintf("\nfound an existing environment named \"%s\"; would you like to overwrite it to connect to this cluster?", envName), "", "")
		}
	}

	if shouldWriteEnv {
		err := addEnvToCLIConfig(newEnvironment)
		if err != nil {
			return err
		}

		if envWasUpdated {
			fmt.Printf(console.Bold("the environment named \"%s\" has been updated to point to this cluster; append `--env %s` to cortex commands to use this cluster (e.g. `cortex deploy --env %s`), or set it as your default with `cortex env default %s`\n"), envName, envName, envName, envName)
		} else {
			fmt.Printf(console.Bold("an environment named \"%s\" has been configured to point to this cluster; append `--env %s` to cortex commands to use this cluster (e.g. `cortex deploy --env %s`), or set it as your default with `cortex env default %s`\n"), envName, envName, envName, envName)
		}
	}

	return nil
}

func createGKECluster(clusterConfig *clusterconfig.GCPConfig, gcpClient *gcp.Client) error {
	fmt.Print("￮ creating GKE cluster ")

	nodeLabels := map[string]string{"workload": "true"}
	var accelerators []*containerpb.AcceleratorConfig

	if clusterConfig.AcceleratorType != nil {
		accelerators = append(accelerators, &containerpb.AcceleratorConfig{
			AcceleratorCount: 1,
			AcceleratorType:  *clusterConfig.AcceleratorType,
		})
		nodeLabels["nvidia.com/gpu"] = "present"
	}

	gkeClusterParent := fmt.Sprintf("projects/%s/locations/%s", *clusterConfig.Project, *clusterConfig.Zone)
	gkeClusterName := fmt.Sprintf("%s/clusters/%s", gkeClusterParent, clusterConfig.ClusterName)

	_, err := gcpClient.CreateCluster(&containerpb.CreateClusterRequest{
		Parent: gkeClusterParent,
		Cluster: &containerpb.Cluster{
			Name:                  clusterConfig.ClusterName,
			InitialClusterVersion: "1.17",
			NodePools: []*containerpb.NodePool{
				{
					Name: "ng-cortex-operator",
					Config: &containerpb.NodeConfig{
						MachineType: "n1-standard-2",
						OauthScopes: []string{
							"https://www.googleapis.com/auth/compute",
							"https://www.googleapis.com/auth/devstorage.read_only",
						},
						ServiceAccount: gcpClient.ClientEmail,
					},
					InitialNodeCount: 1,
				},
				{
					Name: "ng-cortex-worker-on-demand",
					Config: &containerpb.NodeConfig{
						MachineType: *clusterConfig.InstanceType,
						Labels:      nodeLabels,
						Taints: []*containerpb.NodeTaint{
							{
								Key:    "workload",
								Value:  "true",
								Effect: containerpb.NodeTaint_NO_SCHEDULE,
							},
						},
						Accelerators: accelerators,
						OauthScopes: []string{
							"https://www.googleapis.com/auth/compute",
							"https://www.googleapis.com/auth/devstorage.read_only",
						},
						ServiceAccount: gcpClient.ClientEmail,
					},
					Autoscaling: &containerpb.NodePoolAutoscaling{
						Enabled:      true,
						MinNodeCount: int32(*clusterConfig.MinInstances),
						MaxNodeCount: int32(*clusterConfig.MaxInstances),
					},
					InitialNodeCount: int32(*clusterConfig.MinInstances),
				},
			},
			Locations: []string{*clusterConfig.Zone},
		},
	})
	if err != nil {
		return err
	}

	for {
		fmt.Print(".")
		time.Sleep(5 * time.Second)

		cluster, err := gcpClient.GetCluster(gkeClusterName)
		if err != nil {
			return err
		}

		if cluster.Status == containerpb.Cluster_ERROR {
			fmt.Println(" ✗")
			helpStr := fmt.Sprintf("\nyour cluster couldn't be spun up; here is the error that was encountered: %s", cluster.StatusMessage)
			helpStr += fmt.Sprintf("\nadditional error information may be found on the cluster's page in the GCP console: https://console.cloud.google.com/kubernetes/clusters/details/%s/%s?project=%s", *clusterConfig.Zone, clusterConfig.ClusterName, *clusterConfig.Project)
			fmt.Println(helpStr)
			exit.Error(ErrorClusterUp(cluster.StatusMessage))
		}

		if cluster.Status != containerpb.Cluster_PROVISIONING {
			fmt.Println(" ✓")
			break
		}
	}

	return nil
}

func getGCPOperatorLoadBalancerIP(clusterName string, gcpClient *gcp.Client) (string, error) {
	cluster, err := gcpClient.GetCluster(clusterName)
	if err != nil {
		return "", err
	}
	restConfig, err := gcpClient.CreateK8SConfigFromCluster(cluster)
	if err != nil {
		return "", err
	}
	k8sIstio, err := k8s.New("istio-system", false, restConfig)
	if err != nil {
		return "", err
	}
	service, err := k8sIstio.GetService("ingressgateway-operator")
	if err != nil {
		return "", err
	}
	if service == nil {
		return "", ErrorNoOperatorLoadBalancer()
	}

	if len(service.Status.LoadBalancer.Ingress) == 0 {
		return "", errors.ErrorUnexpected("unable to determine operator's endpoint")
	}

	if service.Status.LoadBalancer.Ingress[0].IP == "" {
		return "", errors.ErrorUnexpected("operator's endpoint is missing")
	}

	return service.Status.LoadBalancer.Ingress[0].IP, nil
}
