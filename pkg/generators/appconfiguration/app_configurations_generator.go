// Copyright 2024 KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package appconfiguration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
	yamlv2 "gopkg.in/yaml.v2"
	"gopkg.in/yaml.v3"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sjson "k8s.io/apimachinery/pkg/util/json"
	pkg "kcl-lang.io/kpm/pkg/package"

	"kusionstack.io/kusion-module-framework/pkg/module"
	"kusionstack.io/kusion-module-framework/pkg/module/proto"
	v1 "kusionstack.io/kusion/pkg/apis/api.kusion.io/v1"
	"kusionstack.io/kusion/pkg/engine/runtime/terraform/tfops"
	"kusionstack.io/kusion/pkg/generators"
	"kusionstack.io/kusion/pkg/generators/secret"
	"kusionstack.io/kusion/pkg/log"

	// import the secrets register pkg to register supported secret providers
	ns "kusionstack.io/kusion/pkg/generators/namespace"
	orderedres "kusionstack.io/kusion/pkg/generators/orderedresources"
	_ "kusionstack.io/kusion/pkg/secrets/providers/register"
	jsonutil "kusionstack.io/kusion/pkg/util/json"
	"kusionstack.io/kusion/pkg/workspace"
)

const (
	isWorkload = "kusion.io/is-workload"
	removalVal = "ops://kusionstack.io/remove"
)

const (
	kusionModuleName = "kusion_module_name"
	kusionTraceID    = "kusion_trace_id"
)

type appConfigurationGenerator struct {
	project      *v1.Project
	stack        *v1.Stack
	appName      string
	app          *v1.AppConfiguration
	ws           *v1.Workspace
	dependencies *pkg.Dependencies
}

func NewAppConfigurationGenerator(
	project *v1.Project,
	stack *v1.Stack,
	appName string,
	app *v1.AppConfiguration,
	ws *v1.Workspace,
	dependencies *pkg.Dependencies,
) (generators.SpecGenerator, error) {
	if project == nil {
		return nil, fmt.Errorf("project must not be nil")
	}

	if stack == nil {
		return nil, fmt.Errorf("stack must not be nil")
	}

	if len(appName) == 0 {
		return nil, fmt.Errorf("app name must not be empty")
	}

	if app == nil {
		return nil, fmt.Errorf("can not find app configuration when generating the Spec")
	}

	if ws == nil {
		return nil, errors.New("workspace must not be empty") // AppConfiguration asks for non-empty workspace
	}

	if err := workspace.ValidateWorkspace(ws); err != nil {
		return nil, fmt.Errorf("invalid config of workspace: %s, %w", ws.Name, err)
	}

	return &appConfigurationGenerator{
		project:      project,
		stack:        stack,
		appName:      appName,
		app:          app,
		ws:           ws,
		dependencies: dependencies,
	}, nil
}

func NewAppConfigurationGeneratorFunc(
	project *v1.Project,
	stack *v1.Stack,
	appName string,
	app *v1.AppConfiguration,
	ws *v1.Workspace,
	kpmDependencies *pkg.Dependencies,
) generators.NewSpecGeneratorFunc {
	return func() (generators.SpecGenerator, error) {
		return NewAppConfigurationGenerator(project, stack, appName, app, ws, kpmDependencies)
	}
}

func (g *appConfigurationGenerator) Generate(spec *v1.Spec) error {
	if spec.Resources == nil {
		spec.Resources = make(v1.Resources, 0)
	}
	g.app.Name = g.appName

	// retrieve the module configs of the specified project
	projectModuleConfigs, err := workspace.GetProjectModuleConfigs(g.ws.Modules, g.project.Name)
	if err != nil {
		return err
	}

	// retrieve the imported resources of the specified project
	projectImportedResources := make(map[string]string)
	for _, cfg := range projectModuleConfigs {
		importedResources, err := workspace.GetStringMapFromGenericConfig(cfg, v1.FieldImportedResources)
		if err != nil {
			return err
		}

		for kusionID, importedID := range importedResources {
			if id, ok := projectImportedResources[kusionID]; ok && id != importedID {
				return fmt.Errorf("duplicate kusion id '%s' for importing different resources: '%s' and '%s'",
					kusionID, id, importedID)
			}
			projectImportedResources[kusionID] = importedID
		}
	}

	// generate built-in resources
	namespace := g.getNamespaceName()
	gfs := []generators.NewSpecGeneratorFunc{
		ns.NewNamespaceGeneratorFunc(namespace),
	}

	if g.app.Workload != nil {
		// todo: refactor secret into a module
		gfs = append(gfs, secret.NewSecretGeneratorFunc(&secret.GeneratorRequest{
			Project:     g.project.Name,
			Namespace:   namespace,
			Workload:    g.app.Workload,
			SecretStore: g.ws.SecretStore,
		}))
	}

	if err = generators.CallGenerators(spec, gfs...); err != nil {
		return err
	}

	// call modules to generate customized resources
	wl, resources, patchers, err := g.callModules(projectModuleConfigs)
	if err != nil {
		return err
	}

	// append the generated resources to the spec
	if wl != nil {
		spec.Resources = append(spec.Resources, *wl)
	}
	spec.Resources = append(spec.Resources, resources...)

	// patch workload with resource patchers
	for _, patcher := range patchers {
		if err = PatchWorkload(wl, &patcher); err != nil {
			return err
		}
		if err = JSONPatch(spec.Resources, &patcher); err != nil {
			return err
		}
	}

	// Patch the imported resource IDs to the resource `extensions` in Spec.
	if err = patchImportedResources(spec.Resources, projectImportedResources); err != nil {
		return err
	}

	// The OrderedResourcesGenerator should be executed after all resources are generated.
	if err = generators.CallGenerators(spec, orderedres.NewOrderedResourcesGeneratorFunc()); err != nil {
		return err
	}

	// append secretStore in the Spec
	if g.ws.SecretStore != nil {
		spec.SecretStore = g.ws.SecretStore
	}

	// append context in the Spec
	if g.ws.Context != nil {
		spec.Context = g.ws.Context
	}

	return nil
}

func JSONPatch(resources v1.Resources, patcher *v1.Patcher) error {
	if resources == nil || patcher == nil {
		return nil
	}

	resIndex := resources.Index()

	if patcher.JSONPatchers != nil {
		for id, jsonPatcher := range patcher.JSONPatchers {
			res, ok := resIndex[id]
			if !ok {
				log.Warnf("target patch resource %s not found, skipped", id)
				continue
			}

			target := jsonutil.Marshal2String(res.Attributes)
			switch jsonPatcher.Type {
			case v1.MergePatch:
				modified, err := jsonpatch.MergePatch([]byte(target), jsonPatcher.Payload)
				if err != nil {
					return fmt.Errorf("merge patch to:%s failed with error %w", id, err)
				}
				if err = json.Unmarshal(modified, &res.Attributes); err != nil {
					return err
				}
			case v1.JSONPatch:
				patch, err := jsonpatch.DecodePatch(jsonPatcher.Payload)
				if err != nil {
					return fmt.Errorf("decode json patch:%s failed with error %w", jsonPatcher.Payload, err)
				}

				// Apply JSON Patch with options to allow missing path on `remove`,
				// and ensure path exists on `add`.
				applyOpts := jsonpatch.NewApplyOptions()
				applyOpts.AllowMissingPathOnRemove = true
				applyOpts.EnsurePathExistsOnAdd = true

				modified, err := patch.ApplyWithOptions([]byte(target), applyOpts)
				if err != nil {
					return fmt.Errorf("apply json patch to:%s failed with error %w", id, err)
				}
				if err = json.Unmarshal(modified, &res.Attributes); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported patch type:%s", jsonPatcher.Type)
			}
		}
	}
	return nil
}

func PatchWorkload(workload *v1.Resource, patcher *v1.Patcher) error {
	if patcher == nil {
		return nil
	}

	if workload == nil {
		log.Info("workload is nil, return")
		return nil
	}

	un := &unstructured.Unstructured{}
	attributes := workload.Attributes

	// normalize attributes with K8s json util. Especially numbers are converted to int64 or float64
	out, err := k8sjson.Marshal(attributes)
	if err != nil {
		return err
	}
	if err = k8sjson.Unmarshal(out, &attributes); err != nil {
		return err
	}
	un.SetUnstructuredContent(attributes)

	// patch labels
	if patcher.Labels != nil {
		objLabels := un.GetLabels()
		if objLabels == nil {
			objLabels = make(map[string]string)
		}
		// merge labels
		for k, v := range patcher.Labels {
			// NOTE: we implement value-based map removal by agreeing on a specific value
			// `ops://kusionstack.io/remove` as the `remove` operation index for label patcher.
			if v == removalVal {
				delete(objLabels, k)
				continue
			}

			objLabels[k] = v
		}
		un.SetLabels(objLabels)
	}

	// patch pod labels
	if patcher.PodLabels != nil {
		podLabels, b, err := unstructured.NestedStringMap(un.Object, "spec", "template", "metadata", "labels")
		if err != nil {
			return fmt.Errorf("failed to get pod labels from workload:%s. %w", workload.ID, err)
		}
		if !b || podLabels == nil {
			podLabels = make(map[string]string)
		}
		// merge labels
		for k, v := range patcher.PodLabels {
			// NOTE: we implement value-based map removal by agreeing on a specific value
			// `ops://kusionstack.io/remove` as the `remove` operation index for pod label patcher.
			if v == removalVal {
				delete(podLabels, k)
				continue
			}

			podLabels[k] = v
		}
		err = unstructured.SetNestedStringMap(un.Object, podLabels, "spec", "template", "metadata", "labels")
		if err != nil {
			return err
		}
	}

	// patch annotations
	if patcher.Annotations != nil {
		objAnnotations := un.GetAnnotations()
		if objAnnotations == nil {
			objAnnotations = make(map[string]string)
		}
		// merge annotations
		for k, v := range patcher.Annotations {
			// NOTE: we implement value-based map removal by agreeing on a specific value
			// `ops://kusionstack.io/remove` as the `remove` operation index for annotation patcher.
			if v == removalVal {
				delete(objAnnotations, k)
				continue
			}

			objAnnotations[k] = v
		}
		un.SetAnnotations(objAnnotations)
	}

	// patch pod annotations
	if patcher.PodAnnotations != nil {
		podAnnotations, b, err := unstructured.NestedStringMap(un.Object, "spec", "template", "metadata", "annotations")
		if err != nil {
			return fmt.Errorf("failed to get pod annotations from workload:%s. %w", workload.ID, err)
		}
		if !b || podAnnotations == nil {
			podAnnotations = make(map[string]string)
		}
		// merge annotations
		for k, v := range patcher.PodAnnotations {
			// NOTE: we implement value-based map removal by agreeing on a specific value
			// `ops://kusionstack.io/remove` as the `remove` operation index for pod annotation patcher.
			if v == removalVal {
				delete(podAnnotations, k)
				continue
			}

			podAnnotations[k] = v
		}
		err = unstructured.SetNestedStringMap(un.Object, podAnnotations, "spec", "template", "metadata", "annotations")
		if err != nil {
			return err
		}
	}

	// patch env
	if patcher.Environments != nil {
		containers, b, err := unstructured.NestedSlice(un.Object, "spec", "template", "spec", "containers")
		if err != nil || !b {
			return fmt.Errorf("failed to get containers from workload:%s. %w", workload.ID, err)
		}

		// Split the environment variables to be removed and the ones to be merged.
		// NOTE: we implement value-based array removal by agreeing on a specific value
		// `ops://kusionstack.io/remove` as the `remove` operation index for env patcher.
		var envsToRemove, envsToMerge []k8sv1.EnvVar
		for _, env := range patcher.Environments {
			if env.Value == removalVal {
				envsToRemove = append(envsToRemove, env)
			} else {
				envsToMerge = append(envsToMerge, env)
			}
		}

		for i, c := range containers {
			container := c.(map[string]interface{})
			envs, b, err := unstructured.NestedSlice(container, "env")
			if err != nil {
				return fmt.Errorf("failed to get env from workload:%s, container:%s. %w", workload.ID, container["name"], err)
			}
			if !b {
				envs = make([]interface{}, 0)
			}

			// remove env
			for _, env := range envsToRemove {
				for i := 0; i < len(envs); i++ {
					if e, ok := envs[i].(map[string]interface{}); !ok {
						log.Errorf("failed to assert the unstructured env type: %v", envs[i])
						continue
					} else {
						name := e["name"].(string)
						if name == env.Name {
							envs = append(envs[:i], envs[i+1:]...)
							i--
							log.Infof("we're gonna remove env:%s from workload:%s, container:%s", env.Name, workload.ID,
								container["name"])
						}
					}
				}
			}

			// merge env
			for _, env := range envsToMerge {
				us, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&env)
				if err != nil {
					return err
				}
				// prepend patch env to existing env slices so developers can reference them later on
				// ref: https://kubernetes.io/docs/tasks/inject-data-application/define-interdependent-environment-variables/
				envs = append([]interface{}{us}, envs...)
				log.Infof("we're gonna patch env:%s,value:%s to workload:%s, container:%s", env.Name, env.Value, workload.ID,
					container["name"])
			}

			container["env"] = envs
			containers[i] = container
		}

		if err = unstructured.SetNestedSlice(un.Object, containers, "spec", "template", "spec", "containers"); err != nil {
			return err
		}
	}

	return nil
}

// moduleConfig represents the configuration of a module, either devConfig or platformConfig can be nil
type moduleConfig struct {
	devConfig      v1.Accessory
	platformConfig v1.GenericConfig
	ctx            v1.GenericConfig
}

func (g *appConfigurationGenerator) callModules(projectModuleConfigs map[string]v1.GenericConfig) (workload *v1.Resource, resources []v1.Resource, patchers []v1.Patcher, err error) {
	pluginMap := make(map[string]*module.Plugin)
	defer func() {
		if e := recover(); e != nil {
			switch x := e.(type) {
			case string:
				err = fmt.Errorf("call modules panic:%s", e)
			case error:
				err = x
			default:
				err = errors.New("call modules unknown panic")
			}
		}
		for _, plugin := range pluginMap {
			pluginErr := plugin.KillPluginClient()
			if pluginErr != nil {
				err = fmt.Errorf("kill modules failed %w. %s", err, pluginErr)
			}
		}
		if err != nil {
			log.Errorf(err.Error())
		}
	}()

	if g.dependencies == nil {
		return nil, nil, nil, errors.New("dependencies should not be nil")
	}

	// build module config index
	indexModuleConfig, err := g.buildModuleConfigIndex(projectModuleConfigs)
	if err != nil {
		return nil, nil, nil, err
	}

	workloadKey, err := parseModuleKey(g.app.Workload, g.dependencies)
	if err != nil {
		return nil, nil, nil, err
	}

	// generate customized module resources
	for t, config := range indexModuleConfig {
		response, err := g.invokeModule(pluginMap, t, config)
		if err != nil {
			return nil, nil, nil, err
		}
		// Patch health policy to the resources
		healthPolicy := config.platformConfig[v1.FieldHealthPolicy]
		// parse module result
		// if only one resource exists in the workload module, it is the workload
		if workloadKey == t && len(response.Resources) == 1 {
			workload = &v1.Resource{}
			err = yaml.Unmarshal(response.Resources[0], workload)
			if err != nil {
				return nil, nil, nil, err
			}
			// add isWorkload extension to workload to mark workload
			workload.Extensions[isWorkload] = true
			// Add healthPolicy to workload extensions
			if healthPolicy != nil && workload != nil {
				patchHealthPolicy(workload, healthPolicy)
			}
		} else {
			for _, res := range response.Resources {
				temp := &v1.Resource{}
				err = yaml.Unmarshal(res, temp)
				if err != nil {
					return nil, nil, nil, err
				}
				// filter out workload
				if workloadKey == t && temp.Extensions[isWorkload] == "true" {
					workload = temp
				} else {
					resources = append(resources, *temp)
				}
			}
		}
		if hp, ok := healthPolicy.(v1.GenericConfig); ok {
			for _, res := range resources {
				if res.Type == v1.Kubernetes {
					resAPIVersion, resKind := getAPIVersionKindFromAttributes(res.Attributes)
					hpAPIVersion, hpKind := getAPIVersionKindFromHealthPolicy(hp)
					if strings.EqualFold(resAPIVersion, hpAPIVersion) && strings.EqualFold(resKind, hpKind) {
						patchHealthPolicy(&res, hp)
					}
				}
			}
		}
		// parse patcher
		temp := &v1.Patcher{}
		if response.Patcher != nil {
			err = yaml.Unmarshal(response.Patcher, temp)
			if err != nil {
				return nil, nil, nil, err
			}
			patchers = append(patchers, *temp)
		}
	}

	return workload, resources, patchers, nil
}

func (g *appConfigurationGenerator) invokeModule(
	pluginMap map[string]*module.Plugin,
	key string,
	config moduleConfig,
) (*proto.GeneratorResponse, error) {
	// init the plugin
	if pluginMap[key] == nil {
		plugin, err := module.NewPlugin(key, g.stack.Path)
		if err != nil {
			return nil, err
		}
		if plugin == nil {
			return nil, fmt.Errorf("init plugin for module %s failed", key)
		}
		pluginMap[key] = plugin
	}
	plugin := pluginMap[key]

	// prepare the request
	protoRequest, err := g.initModuleRequest(config)
	if err != nil {
		return nil, err
	}

	// invoke the plugin
	log.Infof("invoke module:%s with request:%s", key, protoRequest.String())
	traceID, _ := uuid.NewUUID()
	ctx := metadata.AppendToOutgoingContext(context.Background(), kusionTraceID, traceID.String(), kusionModuleName, plugin.ModuleName)
	response, err := plugin.Module.Generate(ctx, protoRequest)
	if err != nil {
		return nil, fmt.Errorf("invoke kusion module: %s failed. %w", key, err)
	}
	if response == nil {
		return nil, fmt.Errorf("empty response from module %s", key)
	}
	return response, nil
}

func (g *appConfigurationGenerator) buildModuleConfigIndex(platformModuleConfigs map[string]v1.GenericConfig) (map[string]moduleConfig, error) {
	indexModuleConfig := map[string]moduleConfig{}

	// add workload to the accessory map
	tempMap := make(map[string]v1.Accessory)
	for k, v := range g.app.Accessories {
		tempMap[k] = v
	}
	if g.app.Workload != nil {
		tempMap["workload"] = g.app.Workload
	}

	for accName, accessory := range tempMap {
		// parse accessory module key
		key, err := parseModuleKey(accessory, g.dependencies)
		if err != nil {
			return nil, err
		}
		log.Info("build module index of accessory:%s module key: %s", accName, key)
		moduleName, err := getModuleName(accessory)
		if err != nil {
			return nil, err
		}
		indexModuleConfig[key] = moduleConfig{
			devConfig:      accessory,
			platformConfig: platformModuleConfigs[moduleName],
			ctx:            g.ws.Context,
		}
	}
	return indexModuleConfig, nil
}

// parseModuleKey returns the module key of the accessory in format of "org/module@version"
// example: "kusionstack/mysql@v0.1.0"
func parseModuleKey(accessory v1.Accessory, dependencies *pkg.Dependencies) (string, error) {
	if accessory == nil {
		log.Info("accessory is nil, return empty module key")
		return "", nil
	}

	moduleName, err := getModuleName(accessory)
	if err != nil {
		return "", err
	}

	// find module namespace and version
	d, ok := dependencies.Deps.Get(moduleName)
	if !ok {
		return "", fmt.Errorf("can not find module %s in dependencies", moduleName)
	}
	// key example "kusionstack/mysql@v0.1.0"
	var key string
	if d.Oci != nil {
		key = fmt.Sprintf("%s@%s", d.Oci.Repo, d.Version)
	} else if d.Git != nil {
		// todo: kpm will change the repo version with the filed `version` in d.Git.Version
		url := strings.TrimSuffix(d.Git.Url, ".git")
		splits := strings.Split(url, "/")
		ns := splits[len(splits)-2] + "/" + splits[len(splits)-1]
		key = fmt.Sprintf("%s@%s", ns, d.Git.Tag)
	}
	return key, nil
}

func getModuleName(accessory v1.Accessory) (string, error) {
	t, ok := accessory["_type"]
	if !ok {
		return "", errors.New("can not find '_type' in module config")
	}
	split := strings.Split(t.(string), ".")
	return split[0], nil
}

func (g *appConfigurationGenerator) initModuleRequest(config moduleConfig) (*proto.GeneratorRequest, error) {
	var workloadConfig, secretStoreConfig, devConfig, platformConfig, ctx []byte
	var err error
	// Attention: we MUST yaml.v2 to serialize the object,
	// because we have introduced MapSlice in the Workload which is supported only in the yaml.v2
	if g.app.Workload != nil {
		if workloadConfig, err = yamlv2.Marshal(g.app.Workload); err != nil {
			return nil, fmt.Errorf("marshal workload config failed. %w", err)
		}
	}
	if g.ws.SecretStore != nil {
		if secretStoreConfig, err = yamlv2.Marshal(g.ws.SecretStore); err != nil {
			return nil, fmt.Errorf("marshal secret store config failed. %w", err)
		}
	}
	if config.devConfig != nil {
		if devConfig, err = yaml.Marshal(config.devConfig); err != nil {
			return nil, fmt.Errorf("marshal dev module config failed. %w", err)
		}
	}
	if config.platformConfig != nil {
		if platformConfig, err = yaml.Marshal(config.platformConfig); err != nil {
			return nil, fmt.Errorf("marshal platform module config failed. %w", err)
		}
	}
	if config.ctx != nil {
		if ctx, err = yaml.Marshal(config.ctx); err != nil {
			return nil, fmt.Errorf("marshal context config failed. %w", err)
		}
	}

	protoRequest := &proto.GeneratorRequest{
		Project:        g.project.Name,
		Stack:          g.stack.Name,
		App:            g.appName,
		Workload:       workloadConfig,
		DevConfig:      devConfig,
		PlatformConfig: platformConfig,
		Context:        ctx,
		SecretStore:    secretStoreConfig,
	}
	return protoRequest, nil
}

// getNamespaceName obtains the final namespace name using the following precedence
// (from lower to higher):
// - Project name
// - KubernetesNamespace extensions (specified in corresponding workspace file)
func (g *appConfigurationGenerator) getNamespaceName() string {
	extensions := mergeExtensions(g.project, g.stack)
	if len(extensions) != 0 {
		for _, extension := range extensions {
			switch extension.Kind {
			case v1.KubernetesNamespace:
				return extension.KubeNamespace.Namespace
			default:
				// do nothing
			}
		}
	}

	return g.project.Name
}

func mergeExtensions(project *v1.Project, stack *v1.Stack) []*v1.Extension {
	var extensions []*v1.Extension
	extensionKindMap := make(map[string]struct{})
	if len(stack.Extensions) != 0 {
		for _, extension := range stack.Extensions {
			extensions = append(extensions, extension)
			extensionKindMap[string(extension.Kind)] = struct{}{}
		}
	}
	if len(project.Extensions) != 0 {
		for _, extension := range project.Extensions {
			if _, exist := extensionKindMap[string(extension.Kind)]; !exist {
				extensions = append(extensions, extension)
			}
		}
	}
	return extensions
}

// patchImportedResources patch the imported resource IDs to the `extensions` field
// of the resources in Spec.
func patchImportedResources(resources v1.Resources, projectImportedResources map[string]string) error {
	// Get the map of Kusion ID and Kusion Resource.
	resIndex := resources.Index()

	// Set the `extensions` field of each Kusion Resource.
	for kusionID, importedID := range projectImportedResources {
		if res, ok := resIndex[kusionID]; ok {
			res.Extensions[tfops.ImportIDKey] = importedID
			// remove the resource attribute to avoid update conflict when using terraform import
			res.Attributes = make(map[string]interface{})
		}
	}

	return nil
}

// patchHealthPolicy patch the health policy to the `extensions` field of the resource in the Spec.
func patchHealthPolicy(resource *v1.Resource, healthPolicy any) {
	healthPolicyMap := make(map[string]any)
	if hp, ok := healthPolicy.(v1.GenericConfig); ok {
		for k, v := range hp {
			healthPolicyMap[k] = v
		}
		resource.Extensions[v1.FieldHealthPolicy] = healthPolicyMap
	}
}

// getAPIVersionKindFromAttributes returns the API version and kind from the resource attributes.
func getAPIVersionKindFromAttributes(attributes map[string]interface{}) (apiVersion, kind string) {
	if v, ok := attributes["apiVersion"]; ok {
		apiVersion = v.(string)
	}
	if k, ok := attributes["kind"]; ok {
		kind = k.(string)
	}
	return apiVersion, kind
}

func getAPIVersionKindFromHealthPolicy(healthPolicy v1.GenericConfig) (apiVersion, kind string) {
	if v, ok := healthPolicy["apiVersion"]; ok {
		apiVersion = v.(string)
	}
	if k, ok := healthPolicy["kind"]; ok {
		kind = k.(string)
	}
	return apiVersion, kind
}
