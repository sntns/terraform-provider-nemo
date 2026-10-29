package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/sntns/nemo/pkg/nemo"
	"github.com/sntns/nemo/pkg/progress"
	"github.com/sntns/nemo/pkg/runtime"
	"github.com/sntns/nemo/pkg/stack"
	"github.com/sntns/nemo/pkg/storage"
	"github.com/sntns/nemo/pkg/storage/volume"
)

// owner is the tool nemo records as the manager of the stacks the provider
// writes.
const owner = "terraform"

// up brings the stack to its manifest, as nemo up does.
func up(ctx context.Context, manifest stack.Stack, purge bool) error {
	return withNemo(func(api *nemo.Nemo) error {
		_, err := api.Up(ctx, nemo.UpRequest{Stack: manifest, Purge: purge, Progress: logged(ctx)})
		return err
	})
}

// down uninstalls every installation of the stack, as nemo down does.
func down(ctx context.Context, manifest stack.Stack, purge bool) error {
	return withNemo(func(api *nemo.Nemo) error {
		_, err := api.Down(ctx, nemo.DownRequest{Name: manifest.Name, Manifest: &manifest, Purge: purge, Progress: logged(ctx)})
		return err
	})
}

// withNemo runs the operation with a nemo of its own: the stack's records
// are on its target, nothing is kept where Terraform runs.
func withNemo(operation func(*nemo.Nemo) error) error {
	home, err := os.MkdirTemp("", "terraform-provider-nemo-")
	if err != nil {
		return err
	}
	api, err := nemo.New(nemo.Options{Home: home, Owner: owner})
	if err != nil {
		return errors.Join(err, os.RemoveAll(home))
	}
	return errors.Join(operation(api), os.RemoveAll(home))
}

// installedOn returns the installations of the stack its target has, those
// uninstalled left out.
func installedOn(ctx context.Context, manifest stack.Stack) (_ []storage.Installation, err error) {
	api, err := runtime.Connect(manifest.Target)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, api.Close()) }()
	records, err := volume.Open(ctx, api, manifest.Name, false)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, records.Close(ctx)) }()
	installations, err := records.ListInstallations(ctx, manifest.Name)
	var present []storage.Installation
	for _, installation := range installations {
		if !installation.IsUninstalled() {
			present = append(present, installation)
		}
	}
	return present, err
}

// refreshInstallations brings the model to the installations its target
// has: it drops those uninstalled, sets the version deployed of those that
// give one, and adds those Terraform does not know. A stack just imported
// gets its target and installer from them too.
func refreshInstallations(ctx context.Context, model *nemoStackResourceModel, present []storage.Installation, diags *diag.Diagnostics) {
	if model.Installations == nil && len(present) > 0 {
		model.Target = flattenTarget(ctx, present[0].Target, diags)
		model.Installer = flattenInstaller(present[0].Installer)
		model.PurgeOnDestroy = types.BoolValue(false)
	}
	byName := map[string]storage.Installation{}
	for _, installation := range present {
		byName[installation.Name] = installation
	}
	var installations []nemoStackInstallationModel
	for _, known := range model.Installations {
		installation, ok := byName[known.Name.ValueString()]
		if !ok {
			continue
		}
		if !known.Version.IsNull() {
			known.Version = optionalString(deployedVersion(installation))
		}
		installations = append(installations, known)
		delete(byName, installation.Name)
	}
	for _, installation := range present {
		if _, unknown := byName[installation.Name]; unknown {
			installations = append(installations, flattenInstallation(installation))
		}
	}
	model.Installations = installations
	model.ID = model.Name
}

func encodeManifest(manifest stack.Stack, diags *diag.Diagnostics) types.String {
	encoded, err := stack.Encode(manifest, stack.YAML)
	if err != nil {
		diags.AddError("nemo stack manifest failed", err.Error())
		return types.StringNull()
	}
	return types.StringValue(string(encoded))
}

// bounded is the context of an operation, with the deadline its timeout
// gives it, else none: nemo bounds each action itself.
func bounded(ctx context.Context, timeout func(context.Context, time.Duration) (time.Duration, diag.Diagnostics), diags *diag.Diagnostics) (context.Context, context.CancelFunc) {
	duration, timeoutDiags := timeout(ctx, 0)
	diags.Append(timeoutDiags...)
	if duration <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, duration)
}

// logged writes nemo's progress to Terraform's logs, TF_LOG=INFO.
func logged(ctx context.Context) progress.Sink {
	return progress.SinkFunc(func(event progress.Event) {
		tflog.Info(ctx, event.Group+": "+event.Item, map[string]any{
			"installation": event.Installation, "state": event.State, "message": event.Message,
		})
	})
}

// importID splits an import ID into the Docker's address and the stack's
// name: NAME, or HOST/NAME.
func importID(id string) (string, string, error) {
	host, name := "", id
	if strings.Contains(id, "://") {
		index := strings.LastIndex(id, "/")
		host, name = id[:index], id[index+1:]
	}
	if !dockerName.MatchString(name) {
		return "", "", fmt.Errorf("%q: the stack's name, after the address of its Docker when the environment's is not the one: shop, or tcp://10.1.0.10:2376/shop", id)
	}
	return host, name, nil
}

// importIDOf is the import ID of the stack.
func importIDOf(manifest stack.Stack) string {
	if host := manifest.Target.Host; host != "" {
		return host + "/" + manifest.Name
	}
	return manifest.Name
}
