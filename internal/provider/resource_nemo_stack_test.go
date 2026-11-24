package provider

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sntns/nemo/pkg/runtime"
	"github.com/sntns/nemo/pkg/stack"
	"github.com/sntns/nemo/pkg/storage/volume"
	"github.com/sntns/nemo/pkg/target"
)

func TestNemoStackResource_schema(t *testing.T) {
	t.Parallel()
	var resp fwresource.SchemaResponse
	NewNemoStackResource().Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	assert.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	assert.False(t, resp.Schema.ValidateImplementation(context.Background()).HasError(), "a schema the framework serves")
	assert.True(t, resp.Schema.Attributes["manifest"].IsSensitive(), "it holds the sources of the credentials")
	credential := resp.Schema.Attributes["credentials"].(schema.MapNestedAttribute).NestedObject.Attributes
	assert.True(t, credential["value_wo"].IsWriteOnly(), "a credential's value can stay out of the state")
	key := resp.Schema.Attributes["target"].(schema.SingleNestedAttribute).Attributes["tls"].(schema.SingleNestedAttribute).Attributes["key"].(schema.SingleNestedAttribute)
	assert.NotContains(t, key.Attributes, "value_wo", "every operation reaches the target with its TLS")
}

// A stack on the environment's Docker, with Compose: created, its bundle
// rebuilt at the same path, updated, imported, refused a second time, then
// destroyed.
func TestAccNemoStack_basic(t *testing.T) {
	name := "tf-acc-basic"
	built, err := os.ReadFile(filepath.Join("..", "..", "testdata", "bundles", "hello", "bundle.json"))
	require.NoError(t, err)
	bundle := filepath.Join(t.TempDir(), "bundle.json")
	rebuild := func(version string) {
		var document map[string]any
		require.NoError(t, json.Unmarshal(built, &document))
		document["version"] = version
		data, err := json.Marshal(document)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(bundle, data, 0o600))
	}
	rebuild("0.1.0")
	config := func(test string) string {
		return fmt.Sprintf(loadTestConfiguration(t, "nemo_stack", test), name, bundle)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkNemoStackDestroyed(name),
		Steps: []resource.TestStep{
			{
				Config: config("testAccNemoStack_basic"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("nemo_stack.test", "id", name),
					resource.TestCheckResourceAttr("nemo_stack.test", "target.images", target.ImagesRegistry),
					resource.TestCheckResourceAttrSet("nemo_stack.test", "manifest"),
					resource.TestCheckResourceAttrSet("nemo_stack.test", "content_hash"),
					checkNemoStackInstalled(name, "hello"),
					checkNemoStackDeployed(name, "hello", "0.1.0"),
				),
			},
			{
				PreConfig: func() { rebuild("0.2.0") },
				Config:    config("testAccNemoStack_basic"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("nemo_stack.test", plancheck.ResourceActionUpdate)},
				},
				Check: checkNemoStackDeployed(name, "hello", "0.2.0"),
			},
			{
				Config: config("testAccNemoStack_update"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("nemo_stack.test", "parameters.nemo-timeout", "6m"),
					checkNemoStackInstalled(name, "hello"),
				),
			},
			{
				Config: config("testAccNemoStack_override"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("nemo_stack.test", "installations.0.override", "services:\n  web:\n    environment:\n      GREETING: hello\n"),
					checkServiceEnvironment(name, "web", "GREETING=hello"),
				),
			},
			{
				ResourceName:      "nemo_stack.test",
				ImportState:       true,
				ImportStateVerify: true,
				// The records keep the parameters nemo resolved, not those the configuration gives.
				ImportStateVerifyIgnore: []string{"parameters", "manifest"},
			},
			{
				Config:      config("testAccNemoStack_twin"),
				ExpectError: regexp.MustCompile("nemo stack already exists"),
			},
		},
	})
}

// checkNemoStackInstalled checks that the stack's records on the target
// have these installations, and only these.
func checkNemoStackInstalled(name string, installations ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		present, err := installedOn(context.Background(), stack.Stack{Name: name})
		if err != nil {
			return err
		}
		var names []string
		for _, installation := range present {
			names = append(names, installation.Name)
		}
		if !slices.Equal(names, installations) {
			return fmt.Errorf("stack %s: installed %v, expected %v", name, names, installations)
		}
		return nil
	}
}

// checkNemoStackDeployed checks that the installation's last action ran the
// bundle's version.
func checkNemoStackDeployed(name, installation, version string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		present, err := installedOn(context.Background(), stack.Stack{Name: name})
		if err != nil {
			return err
		}
		for _, installed := range present {
			if installed.Name == installation && deployedVersion(installed) != version {
				return fmt.Errorf("stack %s: %s deployed at %s, expected %s", name, installation, deployedVersion(installed), version)
			}
		}
		return nil
	}
}

// checkServiceEnvironment checks that the containers of the stack's service
// have the variable, as an override gives it.
func checkServiceEnvironment(name, service, variable string) resource.TestCheckFunc {
	return checkServiceContainers(name, service, func(ctx context.Context, api client.APIClient, id string) error {
		inspected, err := api.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
		if err != nil {
			return err
		}
		if !slices.Contains(inspected.Container.Config.Env, variable) {
			return fmt.Errorf("stack %s: %s has not %s", name, service, variable)
		}
		return nil
	})
}

// checkServiceFile checks that the containers of the stack's service have
// the file with this content, as a secret of an override gives it.
func checkServiceFile(name, service, file, content string) resource.TestCheckFunc {
	return checkServiceContainers(name, service, func(ctx context.Context, api client.APIClient, id string) error {
		copied, err := api.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: file})
		if err != nil {
			return err
		}
		defer copied.Content.Close()
		archive := tar.NewReader(copied.Content)
		if _, err := archive.Next(); err != nil {
			return err
		}
		data, err := io.ReadAll(archive)
		if err != nil {
			return err
		}
		if string(data) != content {
			return fmt.Errorf("stack %s: %s has %q in %s, not %q", name, service, data, file, content)
		}
		return nil
	})
}

// checkServiceContainers runs the check on each container of the stack's
// service, of which there must be one at least.
func checkServiceContainers(name, service string, check func(context.Context, client.APIClient, string) error) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		api, err := runtime.Connect(target.Target{})
		if err != nil {
			return err
		}
		defer api.Close()
		containers, err := api.ContainerList(ctx, client.ContainerListOptions{Filters: client.Filters{}.Add("label", "com.docker.compose.project="+name, "com.docker.compose.service="+service)})
		if err != nil {
			return err
		}
		if len(containers.Items) == 0 {
			return fmt.Errorf("stack %s: no container of %s", name, service)
		}
		for _, summary := range containers.Items {
			if err := check(ctx, api, summary.ID); err != nil {
				return err
			}
		}
		return nil
	}
}

// checkNemoStackDestroyed checks that nothing of the stack is installed any
// more, then removes its records from the target.
func checkNemoStackDestroyed(name string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		if err := checkNemoStackInstalled(name)(state); err != nil {
			return err
		}
		api, err := runtime.Connect(target.Target{})
		if err != nil {
			return err
		}
		defer api.Close()
		_, err = api.VolumeRemove(context.Background(), volume.Name(name), client.VolumeRemoveOptions{})
		return err
	}
}

// A credential's value changed behind the same source plans an update, which
// gives the service the new value; one that changes at each read, too.
func TestAccNemoStack_credentials(t *testing.T) {
	name := "tf-acc-credentials"
	bundle, err := filepath.Abs(filepath.Join("..", "..", "testdata", "bundles", "hello", "bundle.json"))
	require.NoError(t, err)
	config := func(source string) string {
		return fmt.Sprintf(loadTestConfiguration(t, "nemo_stack", "testAccNemoStack_credentials"), name, bundle, source)
	}
	variable, minted := `env = "TF_ACC_NEMO_GREETING"`, `command = "od -An -N8 -tx8 /dev/urandom"`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkNemoStackDestroyed(name),
		Steps: []resource.TestStep{
			{
				PreConfig: func() { t.Setenv("TF_ACC_NEMO_GREETING", "hello") },
				Config:    config(variable),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("nemo_stack.test", "content_hash"),
					checkServiceFile(name, "web", "/run/secrets/greeting", "hello"),
				),
			},
			{
				PreConfig: func() { t.Setenv("TF_ACC_NEMO_GREETING", "bonjour") },
				Config:    config(variable),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("nemo_stack.test", plancheck.ResourceActionUpdate)},
				},
				Check: checkServiceFile(name, "web", "/run/secrets/greeting", "bonjour"),
			},
			{Config: config(minted), ExpectNonEmptyPlan: true},
			{
				Config: config(minted),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("nemo_stack.test", plancheck.ResourceActionUpdate)},
				},
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// A bundle that is not the installation's, or not there, fails the plan,
// before anything is deployed.
func TestAccNemoStack_planChecks(t *testing.T) {
	name := "tf-acc-checks"
	bundle, err := filepath.Abs(filepath.Join("..", "..", "testdata", "bundles", "hello", "bundle.json"))
	require.NoError(t, err)
	config := func(test string) string {
		return fmt.Sprintf(loadTestConfiguration(t, "nemo_stack", test), name, bundle)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config("testAccNemoStack_checks"), PlanOnly: true, ExpectError: regexp.MustCompile(`is hello, not other`)},
			{Config: config("testAccNemoStack_checks_missing"), PlanOnly: true, ExpectError: regexp.MustCompile(`nemo stack bundle unreadable`)},
		},
	})
}

// The plan shows the objects the apply will leave, and only their changes:
// none for a parameter alone, a volume an override adds then removes, a
// network a second installation comes to share. A plan that adds or removes
// a bundle.json leaves them to the apply, as content_hash. Each apply leaves
// the objects its plan showed, which the next plan, empty, reads again.
func TestAccNemoStack_objects(t *testing.T) {
	name := "tf-acc-objects"
	hello, bonjour := objectsBundles(t)
	t.Setenv("TF_ACC_NEMO_GREETING", "hello")
	config := func(timeout, override, others string) string {
		return fmt.Sprintf(loadTestConfiguration(t, "nemo_stack", "testAccNemoStack_objects"), name, target.Compose, timeout, hello, override, others)
	}
	withBonjour := fmt.Sprintf("{ name = %q, file = %q },", "bonjour", bonjour)
	network := func(users ...string) knownvalue.Check { return objectOf("network", name+"_default", users...) }

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(state *terraform.State) error {
			// The volume an override declared, then no longer: the upgrade kept it, and a purge removes only those still declared.
			api, err := runtime.Connect(target.Target{})
			if err != nil {
				return err
			}
			defer api.Close()
			_, err = api.VolumeRemove(context.Background(), name+"_data", client.VolumeRemoveOptions{})
			return errors.Join(checkNemoStackDestroyed(name)(state), err)
		},
		Steps: []resource.TestStep{
			{Config: config("5m", "null", ""), ConfigStateChecks: objectsStored(network("hello"))},
			{Config: config("6m", "null", ""), ConfigPlanChecks: objectsPlanned(network("hello")), ConfigStateChecks: objectsStored(network("hello"))},
			{
				Config:            config("6m", strconv.Quote("services:\n  web:\n    volumes: [data:/data]\nvolumes:\n  data: {}\n"), ""),
				ConfigPlanChecks:  objectsPlanned(objectOf("volume", name+"_data", "hello"), network("hello")),
				ConfigStateChecks: objectsStored(objectOf("volume", name+"_data", "hello"), network("hello")),
			},
			{Config: config("6m", "null", ""), ConfigPlanChecks: objectsPlanned(network("hello")), ConfigStateChecks: objectsStored(network("hello"))},
			{Config: config("6m", "null", withBonjour), ConfigPlanChecks: objectsKnownAfterApply(), ConfigStateChecks: objectsStored(network("bonjour", "hello"))},
			{Config: config("7m", "null", withBonjour), ConfigPlanChecks: objectsPlanned(network("bonjour", "hello")), ConfigStateChecks: objectsStored(network("bonjour", "hello"))},
			{Config: config("7m", "null", ""), ConfigPlanChecks: objectsKnownAfterApply(), ConfigStateChecks: objectsStored(network("hello"))},
		},
	})
}

// On Swarm, a secret is an object named after the hash of its content: a
// plan shows it, a value changed behind its source leaves it to the apply,
// which reads that value anew, and a plan shows it removed.
func TestAccNemoStack_objectsOnSwarm(t *testing.T) {
	name := "tf-acc-objects-swarm"
	hello, _ := objectsBundles(t)
	config := func(timeout, override string) string {
		return fmt.Sprintf(loadTestConfiguration(t, "nemo_stack", "testAccNemoStack_objects"), name, target.Swarm, timeout, hello, override, "")
	}
	greeting := strconv.Quote("services:\n  web:\n    secrets: [greeting]\nsecrets:\n  greeting:\n    environment: GREETING\n")
	network := objectOf("network", name+"_default", "hello")
	secret := objectOf("secret", "", "hello")
	secretName := tfjsonpath.New("objects").AtSliceIndex(1).AtMapKey("name")
	renamed := statecheck.CompareValue(compare.ValuesDiffer())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t); testAccSwarmPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkNemoStackDestroyed(name),
		Steps: []resource.TestStep{
			{
				PreConfig:         func() { t.Setenv("TF_ACC_NEMO_GREETING", "hello") },
				Config:            config("5m", greeting),
				ConfigStateChecks: append(objectsStored(network, secret), renamed.AddStateValue("nemo_stack.test", secretName)),
			},
			{Config: config("6m", greeting), ConfigPlanChecks: objectsPlanned(network, secret), ConfigStateChecks: objectsStored(network, secret)},
			{
				PreConfig:         func() { t.Setenv("TF_ACC_NEMO_GREETING", "bonjour") },
				Config:            config("6m", greeting),
				ConfigPlanChecks:  objectsKnownAfterApply(),
				ConfigStateChecks: append(objectsStored(network, secret), renamed.AddStateValue("nemo_stack.test", secretName)),
			},
			{Config: config("6m", "null"), ConfigPlanChecks: objectsPlanned(network), ConfigStateChecks: objectsStored(network)},
		},
	})
}

// testAccSwarmPreCheck skips a test that needs the environment's Docker to
// be a swarm manager.
func testAccSwarmPreCheck(t *testing.T) {
	t.Helper()
	api, err := runtime.Connect(target.Target{})
	require.NoError(t, err)
	defer api.Close()
	info, err := api.Info(context.Background(), client.InfoOptions{})
	require.NoError(t, err)
	if !info.Info.Swarm.ControlAvailable {
		t.Skip("the environment's Docker is not a swarm manager")
	}
}

// objectsBundles are the test bundle, hello, and a copy of it, bonjour,
// whose service is api: two installations of a stack, which share its
// default network.
func objectsBundles(t *testing.T) (string, string) {
	t.Helper()
	hello, err := filepath.Abs(filepath.Join("..", "..", "testdata", "bundles", "hello", "bundle.json"))
	require.NoError(t, err)
	data, err := os.ReadFile(hello)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(data, &document))
	document["name"] = "bonjour"
	project := document["custom"].(map[string]any)["io.nemo.compose"].(map[string]any)["compose"].([]any)[0].(map[string]any)["config"].(map[string]any)
	services := project["services"].(map[string]any)
	services["api"] = services["web"]
	delete(services, "web")
	data, err = json.Marshal(document)
	require.NoError(t, err)
	bonjour := filepath.Join(t.TempDir(), "bundle.json")
	require.NoError(t, os.WriteFile(bonjour, data, 0o600))
	return hello, bonjour
}

// objectOf checks an object of the stack: its kind, its name unless empty,
// and its users.
func objectOf(kind, name string, users ...string) knownvalue.Check {
	var named knownvalue.Check = knownvalue.NotNull()
	if name != "" {
		named = knownvalue.StringExact(name)
	}
	checks := make([]knownvalue.Check, 0, len(users))
	for _, user := range users {
		checks = append(checks, knownvalue.StringExact(user))
	}
	return knownvalue.ObjectExact(map[string]knownvalue.Check{"kind": knownvalue.StringExact(kind), "name": named, "users": knownvalue.ListExact(checks)})
}

// objectsPlanned checks that the plan updates the stack and shows the
// objects it will have.
func objectsPlanned(objects ...knownvalue.Check) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
		plancheck.ExpectResourceAction("nemo_stack.test", plancheck.ResourceActionUpdate),
		plancheck.ExpectKnownValue("nemo_stack.test", tfjsonpath.New("objects"), knownvalue.ListExact(objects)),
	}}
}

// objectsKnownAfterApply checks that the plan updates the stack and leaves
// its objects to the apply.
func objectsKnownAfterApply() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
		plancheck.ExpectResourceAction("nemo_stack.test", plancheck.ResourceActionUpdate),
		plancheck.ExpectUnknownValue("nemo_stack.test", tfjsonpath.New("objects")),
	}}
}

// objectsStored checks the objects the state has after the apply.
func objectsStored(objects ...knownvalue.Check) []statecheck.StateCheck {
	return []statecheck.StateCheck{statecheck.ExpectKnownValue("nemo_stack.test", tfjsonpath.New("objects"), knownvalue.ListExact(objects))}
}
