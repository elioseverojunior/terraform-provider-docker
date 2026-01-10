package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &LogsDataSource{}

type LogsDataSource struct {
	client *docker.Client
}

type LogsDataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Follow         types.Bool   `tfsdk:"follow"`
	Since          types.String `tfsdk:"since"`
	Until          types.String `tfsdk:"until"`
	Tail           types.String `tfsdk:"tail"`
	Timestamps     types.Bool   `tfsdk:"timestamps"`
	ShowStdout     types.Bool   `tfsdk:"show_stdout"`
	ShowStderr     types.Bool   `tfsdk:"show_stderr"`
	LogsRaw        types.String `tfsdk:"logs_raw"`
	DiscardHeaders types.Bool   `tfsdk:"discard_headers"`
}

func NewLogsDataSource() datasource.DataSource {
	return &LogsDataSource{}
}

func (d *LogsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_logs"
}

func (d *LogsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads logs from a Docker container.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this data source.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name or ID of the container to read logs from.",
				Required:    true,
			},
			"follow": schema.BoolAttribute{
				Description: "Follow log output. Default: false. Note: This is only useful in streaming contexts.",
				Optional:    true,
			},
			"since": schema.StringAttribute{
				Description: "Show logs since timestamp (e.g., '2021-01-01T00:00:00Z') or relative time (e.g., '10m').",
				Optional:    true,
			},
			"until": schema.StringAttribute{
				Description: "Show logs until timestamp (e.g., '2021-01-01T00:00:00Z') or relative time (e.g., '10m').",
				Optional:    true,
			},
			"tail": schema.StringAttribute{
				Description: "Number of lines to show from the end of logs. Use 'all' for all lines. Default: 'all'.",
				Optional:    true,
			},
			"timestamps": schema.BoolAttribute{
				Description: "Show timestamps in log output. Default: false.",
				Optional:    true,
			},
			"show_stdout": schema.BoolAttribute{
				Description: "Show stdout logs. Default: true.",
				Optional:    true,
			},
			"show_stderr": schema.BoolAttribute{
				Description: "Show stderr logs. Default: true.",
				Optional:    true,
			},
			"discard_headers": schema.BoolAttribute{
				Description: "Discard the first 8 bytes (Docker log header) from each log line. Default: true.",
				Optional:    true,
			},
			"logs_raw": schema.StringAttribute{
				Description: "The raw log output from the container.",
				Computed:    true,
			},
		},
	}
}

func (d *LogsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *ProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = providerData.DockerClient
}

func (d *LogsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data LogsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	containerName := data.Name.ValueString()

	// Set defaults
	showStdout := true
	if !data.ShowStdout.IsNull() {
		showStdout = data.ShowStdout.ValueBool()
	}

	showStderr := true
	if !data.ShowStderr.IsNull() {
		showStderr = data.ShowStderr.ValueBool()
	}

	discardHeaders := true
	if !data.DiscardHeaders.IsNull() {
		discardHeaders = data.DiscardHeaders.ValueBool()
	}

	tail := "all"
	if !data.Tail.IsNull() && data.Tail.ValueString() != "" {
		tail = data.Tail.ValueString()
	}

	options := container.LogsOptions{
		ShowStdout: showStdout,
		ShowStderr: showStderr,
		Timestamps: data.Timestamps.ValueBool(),
		Follow:     false, // Don't follow in data source - it would block
		Tail:       tail,
	}

	if !data.Since.IsNull() && data.Since.ValueString() != "" {
		options.Since = data.Since.ValueString()
	}

	if !data.Until.IsNull() && data.Until.ValueString() != "" {
		options.Until = data.Until.ValueString()
	}

	logs, err := d.client.ContainerLogs(ctx, containerName, options)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to Read Container Logs",
			fmt.Sprintf("Unable to read logs from container %s: %s", containerName, err),
		)
		return
	}
	defer logs.Close()

	var buf bytes.Buffer
	if discardHeaders {
		// Docker multiplexes stdout/stderr with 8-byte headers
		// We need to strip these headers
		header := make([]byte, 8)
		for {
			_, err := io.ReadFull(logs, header)
			if err != nil {
				if err == io.EOF {
					break
				}
				// If it's not a full 8 bytes, just copy the rest
				buf.Write(header[:])
				io.Copy(&buf, logs)
				break
			}

			// Header format: [STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4]
			size := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])
			if size > 0 {
				line := make([]byte, size)
				_, err := io.ReadFull(logs, line)
				if err != nil {
					break
				}
				buf.Write(line)
			}
		}
	} else {
		io.Copy(&buf, logs)
	}

	data.ID = types.StringValue(fmt.Sprintf("%s-%d", containerName, time.Now().Unix()))
	data.LogsRaw = types.StringValue(buf.String())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
