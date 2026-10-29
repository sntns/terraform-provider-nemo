package provider

import (
	"cmp"
	"context"
	"maps"
	"slices"

	"github.com/cnabio/cnab-go/secrets/host"
	"github.com/cnabio/cnab-go/valuesource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sntns/nemo/pkg/stack"
	"github.com/sntns/nemo/pkg/storage"
	"github.com/sntns/nemo/pkg/target"
)

// expandStack is the stack as nemo reads it, its records on its target.
func expandStack(ctx context.Context, model nemoStackResourceModel, diags *diag.Diagnostics) stack.Stack {
	manifest := stack.Stack{
		Name:        model.Name.ValueString(),
		Target:      expandTarget(ctx, model.Target, diags),
		Installer:   expandInstaller(model.Installer),
		Remote:      model.Remote.ValueString(),
		Records:     stack.RecordsTarget,
		Parameters:  expandStrings(ctx, model.Parameters, diags),
		Credentials: expandCredentials(model.Credentials),
	}
	for _, installation := range model.Installations {
		manifest.Installations = append(manifest.Installations, storage.InstallationSpec{
			Name:  installation.Name.ValueString(),
			Stack: manifest.Name,
			Bundle: storage.BundleReference{
				Repository: installation.Repository.ValueString(),
				Version:    installation.Version.ValueString(),
				File:       installation.File.ValueString(),
			},
			Parameters:  expandStrings(ctx, installation.Parameters, diags),
			Credentials: expandCredentials(installation.Credentials),
		})
	}
	return manifest
}

func expandTarget(ctx context.Context, model *nemoStackTargetModel, diags *diag.Diagnostics) target.Target {
	if model == nil {
		return target.Target{}
	}
	given := target.Target{
		Orchestrator: model.Orchestrator.ValueString(),
		Configuration: target.Configuration{
			Host:    model.Host.ValueString(),
			Context: model.Context.ValueString(),
			Images:  model.Images.ValueString(),
		},
	}
	if tls := model.TLS; tls != nil {
		given.TLS = &target.TLS{CA: expandSource(tls.CA), Cert: expandSource(tls.Cert), Key: expandSource(tls.Key)}
	}
	for name, volume := range model.Volumes {
		if given.Volumes == nil {
			given.Volumes = map[string]target.VolumeType{}
		}
		volumeType := target.VolumeType{VolumeDriver: expandVolumeDriver(ctx, nemoStackVolumeDriverModel{Driver: volume.Driver, Options: volume.Options}, diags)}
		if provision := volume.Provision; provision != nil {
			volumeType.Provision = &target.VolumeProvision{Directory: provision.Directory.ValueString(), Volume: expandVolumeDriver(ctx, provision.Volume, diags)}
		}
		given.Volumes[name] = volumeType
	}
	return given
}

func expandVolumeDriver(ctx context.Context, model nemoStackVolumeDriverModel, diags *diag.Diagnostics) target.VolumeDriver {
	return target.VolumeDriver{Driver: model.Driver.ValueString(), Options: expandStrings(ctx, model.Options, diags)}
}

func expandInstaller(model *nemoStackInstallerModel) target.Installer {
	if model == nil {
		return target.Installer{}
	}
	installer := target.Installer{Driver: model.Driver.ValueString()}
	if kubernetes := model.Kubernetes; kubernetes != nil {
		installer.Kubernetes = &target.Kubernetes{Namespace: kubernetes.Namespace.ValueString(), Kubeconfig: kubernetes.Kubeconfig.ValueString()}
	}
	return installer
}

// expandSource is the one source the validator lets the model give.
func expandSource(model *nemoStackSourceModel) *valuesource.Source {
	if model == nil {
		return nil
	}
	for key, value := range map[string]types.String{host.SourcePath: model.Path, host.SourceEnv: model.Env, host.SourceCommand: model.Command, host.SourceValue: model.Value} {
		if !value.IsNull() {
			return &valuesource.Source{Key: key, Value: value.ValueString()}
		}
	}
	return nil
}

// expandCredentials are the credentials as nemo reads them, by name.
func expandCredentials(credentials map[string]nemoStackSourceModel) []valuesource.Strategy {
	var strategies []valuesource.Strategy
	for _, name := range slices.Sorted(maps.Keys(credentials)) {
		source := credentials[name]
		if read := expandSource(&source); read != nil {
			strategies = append(strategies, valuesource.Strategy{Name: name, Source: *read})
		}
	}
	return strategies
}

func expandStrings(ctx context.Context, value types.Map, diags *diag.Diagnostics) map[string]string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var values map[string]string
	diags.Append(value.ElementsAs(ctx, &values, false)...)
	return values
}

// flattenTarget is the target an installation was made on, as Terraform
// holds it.
func flattenTarget(ctx context.Context, given target.Target, diags *diag.Diagnostics) *nemoStackTargetModel {
	model := &nemoStackTargetModel{
		Orchestrator: optionalString(given.Orchestrator),
		Host:         optionalString(given.Host),
		Context:      optionalString(given.Context),
		Images:       types.StringValue(cmp.Or(given.Images, target.ImagesRegistry)),
	}
	if tls := given.TLS; tls != nil {
		model.TLS = &nemoStackTLSModel{CA: flattenSource(tls.CA), Cert: flattenSource(tls.Cert), Key: flattenSource(tls.Key)}
	}
	for name, volume := range given.Volumes {
		if model.Volumes == nil {
			model.Volumes = map[string]nemoStackVolumeTypeModel{}
		}
		driver := flattenVolumeDriver(ctx, volume.VolumeDriver, diags)
		volumeType := nemoStackVolumeTypeModel{Driver: driver.Driver, Options: driver.Options}
		if provision := volume.Provision; provision != nil {
			volumeType.Provision = &nemoStackVolumeProvisionModel{Directory: types.StringValue(provision.Directory), Volume: flattenVolumeDriver(ctx, provision.Volume, diags)}
		}
		model.Volumes[name] = volumeType
	}
	return model
}

func flattenVolumeDriver(ctx context.Context, driver target.VolumeDriver, diags *diag.Diagnostics) nemoStackVolumeDriverModel {
	return nemoStackVolumeDriverModel{Driver: optionalString(driver.Driver), Options: flattenStrings(ctx, driver.Options, diags)}
}

func flattenInstaller(installer target.Installer) *nemoStackInstallerModel {
	if installer.Driver == "" && installer.Kubernetes == nil {
		return nil
	}
	model := &nemoStackInstallerModel{Driver: optionalString(installer.Driver)}
	if kubernetes := installer.Kubernetes; kubernetes != nil {
		model.Kubernetes = &nemoStackKubernetesModel{Namespace: optionalString(kubernetes.Namespace), Kubeconfig: optionalString(kubernetes.Kubeconfig)}
	}
	return model
}

// flattenInstallation is an installation Terraform does not know: its
// bundle's file, else its repository and the version deployed.
func flattenInstallation(installation storage.Installation) nemoStackInstallationModel {
	model := nemoStackInstallationModel{
		Name:       types.StringValue(installation.Name),
		Version:    types.StringNull(),
		Repository: types.StringNull(),
		File:       types.StringNull(),
		Parameters: types.MapNull(types.StringType),
	}
	if file := installation.Bundle.File; file != "" {
		model.File = types.StringValue(file)
		return model
	}
	model.Repository, model.Version = optionalString(installation.Bundle.Repository), optionalString(deployedVersion(installation))
	return model
}

// deployedVersion is the version of the bundle the installation's last
// action ran.
func deployedVersion(installation storage.Installation) string {
	return cmp.Or(installation.Status.BundleVersion, installation.Bundle.Version)
}

func flattenSource(source *valuesource.Source) *nemoStackSourceModel {
	if source == nil {
		return nil
	}
	model := &nemoStackSourceModel{Path: types.StringNull(), Env: types.StringNull(), Command: types.StringNull(), Value: types.StringNull()}
	value := types.StringValue(source.Value)
	switch source.Key {
	case host.SourcePath:
		model.Path = value
	case host.SourceEnv:
		model.Env = value
	case host.SourceCommand:
		model.Command = value
	default:
		model.Value = value
	}
	return model
}

func flattenStrings(ctx context.Context, values map[string]string, diags *diag.Diagnostics) types.Map {
	if values == nil {
		return types.MapNull(types.StringType)
	}
	value, mapDiags := types.MapValueFrom(ctx, types.StringType, values)
	diags.Append(mapDiags...)
	return value
}

func optionalString(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}
