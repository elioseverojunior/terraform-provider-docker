package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/swarm"
	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &SecretResource{}
	_ resource.ResourceWithImportState = &SecretResource{}
)

type SecretResource struct {
	client *docker.Client
}

type SecretResourceModel struct {
	ID     tftypes.String `tfsdk:"id"`
	Name   tftypes.String `tfsdk:"name"`
	Data   tftypes.String `tfsdk:"data"`
	Labels tftypes.Map    `tfsdk:"labels"`
}

func NewSecretResource() resource.Resource {
	return &SecretResource{}
}

func (r *SecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (r *SecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker Swarm secrets. Requires Docker to be in Swarm mode.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the Docker secret.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"data": schema.StringAttribute{
				Description: "Base64-url-safe-encoded secret data.",
				Required:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"labels": schema.MapAttribute{
				Description: "User-defined key/value metadata.",
				Optional:    true,
				ElementType: tftypes.StringType,
			},
		},
	}
}

func (r *SecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = providerData.DockerClient
}

func (r *SecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SecretResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Decode base64 data
	secretData, err := base64.URLEncoding.DecodeString(data.Data.ValueString())
	if err != nil {
		// Try standard base64 encoding as fallback
		secretData, err = base64.StdEncoding.DecodeString(data.Data.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid Secret Data",
				fmt.Sprintf("Failed to decode base64 secret data: %s", err),
			)
			return
		}
	}

	// Convert labels
	labels := make(map[string]string)
	if !data.Labels.IsNull() {
		resp.Diagnostics.Append(data.Labels.ElementsAs(ctx, &labels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	secretSpec := swarm.SecretSpec{
		Annotations: swarm.Annotations{
			Name:   data.Name.ValueString(),
			Labels: labels,
		},
		Data: secretData,
	}

	tflog.Debug(ctx, "Creating Docker secret", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	secretResponse, err := r.client.SecretCreate(ctx, secretSpec)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Secret Creation Failed",
			fmt.Sprintf("Failed to create secret %s: %s. Note: Docker must be in Swarm mode.", data.Name.ValueString(), err),
		)
		return
	}

	data.ID = tftypes.StringValue(secretResponse.ID)

	tflog.Debug(ctx, "Created Docker secret", map[string]interface{}{
		"id":   secretResponse.ID,
		"name": data.Name.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SecretResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, _, err := r.client.SecretInspectWithRaw(ctx, data.ID.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "No such secret") || strings.Contains(err.Error(), "secret not found") {
			tflog.Debug(ctx, "Secret not found, removing from state", map[string]interface{}{
				"id": data.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Docker Secret Read Failed",
			fmt.Sprintf("Failed to read secret %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	data.Name = tftypes.StringValue(secret.Spec.Name)
	// Note: Secret data is never returned by Docker API after creation

	if len(secret.Spec.Labels) > 0 {
		labels, diags := tftypes.MapValueFrom(ctx, tftypes.StringType, secret.Spec.Labels)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Labels = labels
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SecretResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current secret to get version
	secret, _, err := r.client.SecretInspectWithRaw(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Secret Update Failed",
			fmt.Sprintf("Failed to inspect secret %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	// Convert labels
	labels := make(map[string]string)
	if !data.Labels.IsNull() {
		resp.Diagnostics.Append(data.Labels.ElementsAs(ctx, &labels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Only labels can be updated, data changes require replacement
	secretSpec := swarm.SecretSpec{
		Annotations: swarm.Annotations{
			Name:   data.Name.ValueString(),
			Labels: labels,
		},
	}

	err = r.client.SecretUpdate(ctx, data.ID.ValueString(), secret.Version, secretSpec)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Secret Update Failed",
			fmt.Sprintf("Failed to update secret %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SecretResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting Docker secret", map[string]interface{}{
		"id":   data.ID.ValueString(),
		"name": data.Name.ValueString(),
	})

	err := r.client.SecretRemove(ctx, data.ID.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "No such secret") || strings.Contains(err.Error(), "secret not found") {
			tflog.Debug(ctx, "Secret already removed", map[string]interface{}{
				"id": data.ID.ValueString(),
			})
			return
		}
		resp.Diagnostics.AddError(
			"Docker Secret Deletion Failed",
			fmt.Sprintf("Failed to delete secret %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	tflog.Debug(ctx, "Deleted Docker secret", map[string]interface{}{
		"id": data.ID.ValueString(),
	})
}

func (r *SecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by ID or name
	secret, _, err := r.client.SecretInspectWithRaw(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Secret Import Failed",
			fmt.Sprintf("Failed to import secret %s: %s. Note: Secret data cannot be imported as it is never exposed after creation.", req.ID, err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), secret.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), secret.Spec.Name)...)
	// Data must be provided manually after import since Docker never exposes secret data
	resp.Diagnostics.AddWarning(
		"Secret Data Required",
		"The secret data must be provided in the Terraform configuration as Docker does not expose secret data after creation.",
	)
}
