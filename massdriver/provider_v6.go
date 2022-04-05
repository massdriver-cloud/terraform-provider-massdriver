package massdriver

import (
	"context"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type MassdriverProvider struct {
	client MassdriverClient
}

func (m MassdriverProvider) ApplyResourceChange(context.Context, *tfprotov6.ApplyResourceChangeRequest) (*tfprotov6.ApplyResourceChangeResponse, error) {
	event := NewEvent(EVENT_TYPE_ARTIFACT_CREATED)
	m.client.PublishEventToSNS(event)

	return &tfprotov6.ApplyResourceChangeResponse{
		NewState:                    &tfprotov6.DynamicValue{},
		Private:                     []byte{},
		Diagnostics:                 []*tfprotov6.Diagnostic{},
		UnsafeToUseLegacyTypeSystem: false,
	}, nil
}

func (m MassdriverProvider) ConfigureProvider(ctx context.Context, request *tfprotov6.ConfigureProviderRequest) (*tfprotov6.ConfigureProviderResponse, error) {
	config, err := request.Config.Unmarshal(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"deployment_id":   tftypes.String,
			"token":           tftypes.String,
			"event_topic_arn": tftypes.String,
		},
		OptionalAttributes: map[string]struct{}{},
	})
	goMap := make(map[string]string)
	config.As(&goMap)

	var diags []*tfprotov6.Diagnostic
	c, err := NewMassdriverClient(goMap["deployment_id"], goMap["token"], goMap["event_topic_arn"])
	if err != nil {
		diags = append(diags, &tfprotov6.Diagnostic{
			Severity:  0,
			Summary:   "Unable to create Massdriver client",
			Detail:    err.Error(),
			Attribute: &tftypes.AttributePath{},
		})
	}

	// store the client on the struct
	m.client = *c

	return &tfprotov6.ConfigureProviderResponse{
		Diagnostics: diags,
	}, err
}

func (m MassdriverProvider) GetProviderSchema(context.Context, *tfprotov6.GetProviderSchemaRequest) (*tfprotov6.GetProviderSchemaResponse, error) {
	return &tfprotov6.GetProviderSchemaResponse{
		Provider: &tfprotov6.Schema{
			Block: &tfprotov6.SchemaBlock{
				Attributes: []*tfprotov6.SchemaAttribute{
					{Name: "deployment_id", Type: tftypes.String, Required: true},
					{Name: "token", Type: tftypes.String, Required: true, Sensitive: true},
					{Name: "event_topic_arn", Type: tftypes.String, Required: true},
				}}},
		ProviderMeta: &tfprotov6.Schema{},
		ResourceSchemas: map[string]*tfprotov6.Schema{
			"massdriver_artifact": {
				Block: &tfprotov6.SchemaBlock{
					Attributes: []*tfprotov6.SchemaAttribute{
						{Name: "last_updated", Type: tftypes.String, Optional: true, Computed: true},
						{Name: "artifact", Type: tftypes.String, Required: true, Sensitive: true},
						{Name: "type", Type: tftypes.String, Required: true, Sensitive: true},
						{Name: "field", Type: tftypes.String, Required: true, Sensitive: true},
					}}},
		},
		DataSourceSchemas: map[string]*tfprotov6.Schema{},
		Diagnostics:       []*tfprotov6.Diagnostic{},
	}, nil
}

func (m MassdriverProvider) ImportResourceState(context.Context, *tfprotov6.ImportResourceStateRequest) (*tfprotov6.ImportResourceStateResponse, error) {
	return &tfprotov6.ImportResourceStateResponse{
		ImportedResources: []*tfprotov6.ImportedResource{},
		Diagnostics:       []*tfprotov6.Diagnostic{},
	}, nil
}

func (m MassdriverProvider) PlanResourceChange(context.Context, *tfprotov6.PlanResourceChangeRequest) (*tfprotov6.PlanResourceChangeResponse, error) {
	return &tfprotov6.PlanResourceChangeResponse{
		PlannedState:                &tfprotov6.DynamicValue{},
		RequiresReplace:             []*tftypes.AttributePath{},
		PlannedPrivate:              []byte{},
		Diagnostics:                 []*tfprotov6.Diagnostic{},
		UnsafeToUseLegacyTypeSystem: false,
	}, nil
}

func (m MassdriverProvider) ReadDataSource(context.Context, *tfprotov6.ReadDataSourceRequest) (*tfprotov6.ReadDataSourceResponse, error) {
	return &tfprotov6.ReadDataSourceResponse{
		State:       &tfprotov6.DynamicValue{},
		Diagnostics: []*tfprotov6.Diagnostic{},
	}, nil
}

func (m MassdriverProvider) ReadResource(context.Context, *tfprotov6.ReadResourceRequest) (*tfprotov6.ReadResourceResponse, error) {
	return &tfprotov6.ReadResourceResponse{
		NewState:    &tfprotov6.DynamicValue{},
		Diagnostics: []*tfprotov6.Diagnostic{},
		Private:     []byte{},
	}, nil
}

func (m MassdriverProvider) StopProvider(context.Context, *tfprotov6.StopProviderRequest) (*tfprotov6.StopProviderResponse, error) {
	return &tfprotov6.StopProviderResponse{
		Error: "",
	}, nil
}

func (m MassdriverProvider) UpgradeResourceState(context.Context, *tfprotov6.UpgradeResourceStateRequest) (*tfprotov6.UpgradeResourceStateResponse, error) {
	return &tfprotov6.UpgradeResourceStateResponse{
		UpgradedState: &tfprotov6.DynamicValue{},
		Diagnostics:   []*tfprotov6.Diagnostic{},
	}, nil
}

func (m MassdriverProvider) ValidateDataResourceConfig(context.Context, *tfprotov6.ValidateDataResourceConfigRequest) (*tfprotov6.ValidateDataResourceConfigResponse, error) {
	return &tfprotov6.ValidateDataResourceConfigResponse{
		Diagnostics: []*tfprotov6.Diagnostic{},
	}, nil
}

func (m MassdriverProvider) ValidateProviderConfig(ctx context.Context, request *tfprotov6.ValidateProviderConfigRequest) (*tfprotov6.ValidateProviderConfigResponse, error) {
	return &tfprotov6.ValidateProviderConfigResponse{
		PreparedConfig: request.Config,
		Diagnostics:    []*tfprotov6.Diagnostic{},
	}, nil
}

func (m MassdriverProvider) ValidateResourceConfig(context.Context, *tfprotov6.ValidateResourceConfigRequest) (*tfprotov6.ValidateResourceConfigResponse, error) {
	return &tfprotov6.ValidateResourceConfigResponse{
		Diagnostics: []*tfprotov6.Diagnostic{},
	}, nil
}

func ProviderServer() tfprotov6.ProviderServer {
	return MassdriverProvider{
		// client: &MassdriverClient{},
	}
}
