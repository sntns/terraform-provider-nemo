package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/stretchr/testify/assert"
)

func TestStackResource_schema(t *testing.T) {
	t.Parallel()
	var response resource.SchemaResponse
	newStackResource().Schema(context.Background(), resource.SchemaRequest{}, &response)
	assert.False(t, response.Diagnostics.HasError(), response.Diagnostics)
	assert.False(t, response.Schema.ValidateImplementation(context.Background()).HasError(), "a schema the framework serves")
	assert.True(t, response.Schema.Attributes["manifest"].IsSensitive(), "it holds the credentials given by value")
}
