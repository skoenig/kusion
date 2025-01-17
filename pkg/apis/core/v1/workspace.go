package v1

const (
	DefaultBlock         = "default"
	ProjectSelectorField = "projectSelector"
)

// Workspace is a logical concept representing a target that stacks will be deployed to.
//
// Workspace is managed by platform engineers, which contains a set of configurations
// that application developers do not want or should not concern, and is reused by multiple
// stacks belonging to different projects.
type Workspace struct {
	// Name identifies a Workspace uniquely.
	Name string `yaml:"-" json:"-"`

	// Modules are the configs of a set of modules.
	Modules ModuleConfigs `yaml:"modules,omitempty" json:"modules,omitempty"`

	// Runtimes are the configs of a set of runtimes.
	Runtimes *RuntimeConfigs `yaml:"runtimes,omitempty" json:"runtimes,omitempty"`

	// SecretStore represents a secure external location for storing secrets.
	SecretStore *SecretStoreSpec `yaml:"secretStore,omitempty" json:"secretStore,omitempty"`
}

// ModuleConfigs is a set of multiple ModuleConfig, whose key is the module name.
// The module name format is "source@version".
type ModuleConfigs map[string]*ModuleConfig

// ModuleConfig is the config of a module, which contains a default and several patcher blocks.
//
// The default block's key is "default", and value is the module inputs. The patcher blocks' keys
// are the patcher names, which are just block identifiers without specific meaning, but must
// not be "default". Besides module inputs, patcher block's value also contains a field named
// "projectSelector", whose value is a slice containing the project names that use the patcher
// configs. A project can only be assigned in a patcher's "projectSelector" field, the assignment
// in multiple patchers is not allowed. For a project, if not specified in the patcher block's
// "projectSelector" field, the default config will be used.
//
// Take the ModuleConfig of "database" for an example, which is shown as below:
//
//	 config := ModuleConfig {
//		"default": {
//			"type":         "aws",
//			"version":      "5.7",
//			"instanceType": "db.t3.micro",
//		},
//		"smallClass": {
//		 	"instanceType":    "db.t3.small",
//		 	"projectSelector": []string{"foo", "bar"},
//		},
//	}
type ModuleConfig struct {
	// Default is default block of the module config.
	Default GenericConfig `yaml:"default" json:"default"`

	// ModulePatcherConfigs are the patcher blocks of the module config.
	ModulePatcherConfigs `yaml:",inline,omitempty" json:",inline,omitempty"`
}

// ModulePatcherConfigs is a group of ModulePatcherConfig.
type ModulePatcherConfigs map[string]*ModulePatcherConfig

// ModulePatcherConfig is a patcher block of the module config.
type ModulePatcherConfig struct {
	// GenericConfig contains the module configs.
	GenericConfig `yaml:",inline" json:",inline"`

	// ProjectSelector contains the selected projects.
	ProjectSelector []string `yaml:"projectSelector" json:"projectSelector"`
}

// RuntimeConfigs contains a set of runtime config.
type RuntimeConfigs struct {
	// Kubernetes contains the config to access a kubernetes cluster.
	Kubernetes *KubernetesConfig `yaml:"kubernetes,omitempty" json:"kubernetes,omitempty"`

	// Terraform contains the config of multiple terraform providers.
	Terraform TerraformConfig `yaml:"terraform,omitempty" json:"terraform,omitempty"`
}

// KubernetesConfig contains config to access a kubernetes cluster.
type KubernetesConfig struct {
	// KubeConfig is the path of the kubeconfig file.
	KubeConfig string `yaml:"kubeConfig" json:"kubeConfig"`
}

// TerraformConfig contains the config of multiple terraform provider config, whose key is
// the provider name.
type TerraformConfig map[string]*ProviderConfig

// ProviderConfig contains the full configurations of a specified provider. It is the combination
// of the specified provider's config in blocks "terraform/required_providers" and "providers" in
// terraform hcl file, where the former is described by fields Source and Version, and the latter
// is described by GenericConfig cause different provider has different config.
type ProviderConfig struct {
	// Source of the provider.
	Source string `yaml:"source" json:"source"`

	// Version of the provider.
	Version string `yaml:"version" json:"version"`

	// GenericConfig is used to describe the config of a specified terraform provider.
	GenericConfig `yaml:",inline,omitempty" json:",inline,omitempty"`
}

// GenericConfig is a generic model to describe config which shields the difference among multiple concrete
// models. GenericConfig is designed for extensibility, used for module, terraform runtime config, etc.
type GenericConfig map[string]any

type VaultKVStoreVersion string

const (
	VaultKVStoreV1 VaultKVStoreVersion = "v1"
	VaultKVStoreV2 VaultKVStoreVersion = "v2"
)

// ExternalSecretRef contains information that points to the secret store data location.
type ExternalSecretRef struct {
	// Specifies the name of the secret in Provider to read, mandatory.
	Name string `yaml:"name" json:"name"`

	// Specifies the version of the secret to return, if supported.
	Version string `yaml:"version,omitempty" json:"version,omitempty"`

	// Used to select a specific property of the secret data (if a map), if supported.
	Property string `yaml:"property,omitempty" json:"property,omitempty"`
}

// SecretStoreSpec contains configuration to describe target secret store.
type SecretStoreSpec struct {
	Provider *ProviderSpec `yaml:"provider" json:"provider"`
}

// ProviderSpec contains provider-specific configuration.
type ProviderSpec struct {
	// Alicloud configures a store to retrieve secrets from Alicloud Secrets Manager.
	Alicloud *AlicloudProvider `yaml:"alicloud,omitempty" json:"alicloud,omitempty"`

	// AWS configures a store to retrieve secrets from AWS Secrets Manager.
	AWS *AWSProvider `yaml:"aws,omitempty" json:"aws,omitempty"`

	// Vault configures a store to retrieve secrets from HashiCorp Vault.
	Vault *VaultProvider `yaml:"vault,omitempty" json:"vault,omitempty"`

	// Azure configures a store to retrieve secrets from Azure KeyVault.
	Azure *AzureKVProvider `yaml:"azure,omitempty" json:"azure,omitempty"`

	// Fake configures a store with static key/value pairs
	Fake *FakeProvider `yaml:"fake,omitempty" json:"fake,omitempty"`
}

// AlicloudProvider configures a store to retrieve secrets from Alicloud Secrets Manager.
type AlicloudProvider struct {
	// Alicloud Region to be used to interact with Alicloud Secrets Manager.
	// Examples are cn-beijing, cn-shanghai, etc.
	Region string `yaml:"region" json:"region"`
}

// AWSProvider configures a store to retrieve secrets from AWS Secrets Manager.
type AWSProvider struct {
	// AWS Region to be used to interact with AWS Secrets Manager.
	// Examples are us-east-1, us-west-2, etc.
	Region string `yaml:"region" json:"region"`

	// The profile to be used to interact with AWS Secrets Manager.
	// If not set, the default profile created with `aws configure` will be used.
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
}

// VaultProvider configures a store to retrieve secrets from HashiCorp Vault.
type VaultProvider struct {
	// Server is the target Vault server address to connect, e.g: "https://vault.example.com:8200".
	Server string `yaml:"server" json:"server"`

	// Path is the mount path of the Vault KV backend endpoint, e.g: "secret".
	Path *string `yaml:"path,omitempty" json:"path,omitempty"`

	// Version is the Vault KV secret engine version. Version can be either "v1" or
	// "v2", defaults to "v2".
	Version VaultKVStoreVersion `yaml:"version" json:"version"`
}

// AzureEnvironmentType specifies the Azure cloud environment endpoints to use for connecting and authenticating with Azure.
type AzureEnvironmentType string

const (
	AzureEnvironmentPublicCloud       AzureEnvironmentType = "PublicCloud"
	AzureEnvironmentUSGovernmentCloud AzureEnvironmentType = "USGovernmentCloud"
	AzureEnvironmentChinaCloud        AzureEnvironmentType = "ChinaCloud"
	AzureEnvironmentGermanCloud       AzureEnvironmentType = "GermanCloud"
)

// AzureKVProvider configures a store to retrieve secrets from Azure KeyVault
type AzureKVProvider struct {
	// Vault Url from which the secrets to be fetched from.
	VaultURL *string `yaml:"vaultUrl" json:"vaultUrl"`

	// TenantID configures the Azure Tenant to send requests to.
	TenantID *string `yaml:"tenantId" json:"tenantId"`

	// EnvironmentType specifies the Azure cloud environment endpoints to use for connecting and authenticating with Azure.
	// By-default it points to the public cloud AAD endpoint, and the following endpoints are available:
	// PublicCloud, USGovernmentCloud, ChinaCloud, GermanCloud
	// Ref: https://github.com/Azure/go-autorest/blob/main/autorest/azure/environments.go#L152
	EnvironmentType AzureEnvironmentType `yaml:"environmentType,omitempty" json:"environmentType,omitempty"`
}

// FakeProvider configures a fake provider that returns static values.
type FakeProvider struct {
	Data []FakeProviderData `json:"data"`
}

type FakeProviderData struct {
	Key      string            `json:"key"`
	Value    string            `json:"value,omitempty"`
	ValueMap map[string]string `json:"valueMap,omitempty"`
	Version  string            `json:"version,omitempty"`
}
