package massdriver

import (
	"context"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// might have to loop through schema artifact provider
// loop through artifacts, validate

type Mything struct {
}

func (m Mything) ApplyResourceChange(context.Context, *tfprotov6.ApplyResourceChangeRequest) (*tfprotov6.ApplyResourceChangeResponse, error) {
	return &tfprotov6.ApplyResourceChangeResponse{}, nil
}

func (m Mything) ConfigureProvider(ctx context.Context, request *tfprotov6.ConfigureProviderRequest) (*tfprotov6.ConfigureProviderResponse, error) {
	deployment_id, err := request.Config.Unmarshal(tftypes.String) //  tftypes.Type{"deployment_id"})
	token, err := request.Config.Unmarshal(tftypes.String)
	eventTopicARN, err := request.Config.Unmarshal(tftypes.String) //, "event_topic_arn")

	// var diags diag.Diagnostics;

	// c, err := NewMassdriverClient(deployment_id, token, eventTopicARN)

	// .Config.Get("deployment_id").(string)

	// Unmarshal

	return &tfprotov6.ConfigureProviderResponse{

		// token := d.Get("token").(string)
		// eventTopicARN := d.Get("event_topic_arn").(string)

		// var diags diag.Diagnostics
		// c, err := NewMassdriverClient(deployment_id, token, eventTopicARN)
		// if err != nil {
		// 	diags = append(diags, diag.Diagnostic{
		// 		Severity: diag.Error,
		// 		Summary:  "Unable to create Massdriver client",
		// 		Detail:   err.Error(),
		// 	})
		// 	return nil, diags
		// }
	}, nil
}

func (m Mything) GetProviderSchema(context.Context, *tfprotov6.GetProviderSchemaRequest) (*tfprotov6.GetProviderSchemaResponse, error) {
	return &tfprotov6.GetProviderSchemaResponse{
		Provider: &tfprotov6.Schema{
			Block: &tfprotov6.SchemaBlock{
				Attributes: []*tfprotov6.SchemaAttribute{
					{
						Name:     "deployment_id",
						Type:     tftypes.String,
						Required: true,
						// DefaultFunc: schema.EnvDefaultFunc("MASSDRIVER_DEPLOYMENT_ID", nil),
					},
					{
						Name:      "token",
						Type:      tftypes.String,
						Required:  true,
						Sensitive: true,
						// DefaultFunc: schema.EnvDefaultFunc("MASSDRIVER_TOKEN", nil),
					},
					{
						Name:     "event_topic_arn",
						Type:     tftypes.String,
						Required: true,
						// DefaultFunc: schema.EnvDefaultFunc("MASSDRIVER_EVENT_TOPIC_ARN", nil),
					},
				},
			},
		},
		ResourceSchemas: map[string]*tfprotov6.Schema{
			"massdriver_artifact": {
				Block: &tfprotov6.SchemaBlock{
					Attributes: []*tfprotov6.SchemaAttribute{
						{
							Name:     "last_updated",
							Type:     tftypes.String,
							Optional: true,
							Computed: true,
						},
						{
							Name:      "artifact",
							Type:      tftypes.String,
							Required:  true,
							Sensitive: true,
						},
					},
				},
			},
		},
	}, nil
}

func (m Mything) ImportResourceState(context.Context, *tfprotov6.ImportResourceStateRequest) (*tfprotov6.ImportResourceStateResponse, error) {
	return &tfprotov6.ImportResourceStateResponse{}, nil
}

func (m Mything) PlanResourceChange(context.Context, *tfprotov6.PlanResourceChangeRequest) (*tfprotov6.PlanResourceChangeResponse, error) {
	return &tfprotov6.PlanResourceChangeResponse{}, nil
}

func (m Mything) ReadDataSource(context.Context, *tfprotov6.ReadDataSourceRequest) (*tfprotov6.ReadDataSourceResponse, error) {
	return &tfprotov6.ReadDataSourceResponse{}, nil
}

func (m Mything) ReadResource(context.Context, *tfprotov6.ReadResourceRequest) (*tfprotov6.ReadResourceResponse, error) {
	return &tfprotov6.ReadResourceResponse{}, nil
}

func (m Mything) StopProvider(context.Context, *tfprotov6.StopProviderRequest) (*tfprotov6.StopProviderResponse, error) {
	return &tfprotov6.StopProviderResponse{}, nil
}

func (m Mything) UpgradeResourceState(context.Context, *tfprotov6.UpgradeResourceStateRequest) (*tfprotov6.UpgradeResourceStateResponse, error) {
	return &tfprotov6.UpgradeResourceStateResponse{}, nil
}

func (m Mything) ValidateDataResourceConfig(context.Context, *tfprotov6.ValidateDataResourceConfigRequest) (*tfprotov6.ValidateDataResourceConfigResponse, error) {
	return &tfprotov6.ValidateDataResourceConfigResponse{}, nil
}

func (m Mything) ValidateProviderConfig(context.Context, *tfprotov6.ValidateProviderConfigRequest) (*tfprotov6.ValidateProviderConfigResponse, error) {
	return &tfprotov6.ValidateProviderConfigResponse{}, nil
}

func (m Mything) ValidateResourceConfig(context.Context, *tfprotov6.ValidateResourceConfigRequest) (*tfprotov6.ValidateResourceConfigResponse, error) {
	return &tfprotov6.ValidateResourceConfigResponse{}, nil
}

func ProviderServer() tfprotov6.ProviderServer {
	return Mything{}
}
