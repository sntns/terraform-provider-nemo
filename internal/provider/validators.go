package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// oneSource checks that a credential is read from one source exactly.
type oneSource struct{}

var sources = []string{"path", "env", "command", "value"}

func (oneSource) Description(context.Context) string {
	return "exactly one of " + strings.Join(sources, ", ")
}

func (validator oneSource) MarkdownDescription(ctx context.Context) string {
	return validator.Description(ctx)
}

func (oneSource) ValidateObject(_ context.Context, request validator.ObjectRequest, response *validator.ObjectResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}
	var given []string
	for _, name := range sources {
		if value, ok := request.ConfigValue.Attributes()[name]; ok && (!value.IsNull() || value.IsUnknown()) {
			given = append(given, name)
		}
	}
	if len(given) != 1 {
		response.Diagnostics.AddAttributeError(request.Path, "One source of a credential",
			fmt.Sprintf("give one of %s, not %d", strings.Join(sources, ", "), len(given)))
	}
}
