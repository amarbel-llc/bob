mod app;
mod background;
mod config;
mod lsp_client;
mod nix_runner;
mod output;
mod resources;
mod tools;
mod validators;

use mcp_server::server::{McpServerBuilder, run_stdio_server};
use std::io::Read as _;
use std::process::Command;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let args: Vec<String> = std::env::args().collect();

    match args.get(1).map(|s| s.as_str()) {
        Some("generate-plugin") => {
            let mut app = app::make_app();
            let remaining: Vec<String> = args[2..].to_vec();
            app.handle_generate_plugin(&remaining, &mut std::io::stdout())?;
            Ok(())
        }
        Some("hook") => {
            let mut input = Vec::new();
            std::io::stdin().read_to_end(&mut input)?;
            let app = app::make_app();
            if let Some(output) = app.handle_hook(&input)? {
                std::io::Write::write_all(&mut std::io::stdout(), &output)?;
            }
            Ok(())
        }
        Some("install-claude") => install_claude(),
        _ => run_server().await,
    }
}

fn install_claude() -> anyhow::Result<()> {
    let exe_path = std::env::current_exe()?;

    // Remove existing chix MCP server (ignore errors if it doesn't exist)
    let _ = Command::new("claude")
        .args(["mcp", "remove", "chix"])
        .status();

    // Add chix MCP server
    let status = Command::new("claude")
        .args(["mcp", "add", "chix", "--", exe_path.to_str().unwrap()])
        .status()?;

    if status.success() {
        println!("Successfully installed chix as MCP server 'chix'");
        println!("To verify, run: claude mcp list");
        Ok(())
    } else {
        anyhow::bail!("Failed to install MCP server");
    }
}

async fn run_server() -> anyhow::Result<()> {
    let server = McpServerBuilder::new("chix", "0.1.0")
        .instructions("Nix MCP server providing tools for building, evaluating, and managing Nix flakes, packages, and store paths. Includes FlakeHub and Cachix integration, Nix language diagnostics via nil LSP, and background task management.")
        // Tools (mutating or interactive operations remain as tools)
        .with_tool_v1(tools::BuildTool)
        .with_tool_v1(tools::FlakeCheckTool)
        .with_tool_v1(tools::FlakeUpdateTool)
        .with_tool_v1(tools::FlakeLockTool)
        .with_tool_v1(tools::FlakeInitTool)
        .with_tool_v1(tools::RunTool)
        .with_tool_v1(tools::DevelopRunTool)
        .with_tool_v1(tools::SearchTool)
        .with_tool_v1(tools::StoreGcTool)
        .with_tool_v1(tools::CopyTool)
        .with_tool_v1(tools::EvalTool)
        .with_tool_v1(tools::FhAddTool)
        .with_tool_v1(tools::FhFetchTool)
        .with_tool_v1(tools::FhLoginTool)
        .with_tool_v1(tools::CachixPushTool)
        .with_tool_v1(tools::CachixUseTool)
        .with_tool_v1(tools::TaskStatusTool)
        .with_tool_v1(tools::NilCompletionsTool)
        // Resource access tools (for subagent access)
        .with_tool_v1(tools::ResourceTemplatesTool)
        .with_tool_v1(tools::ResourceReadTool)
        // Resources (read-only operations migrated from tools)
        .with_resource(resources::BuildLogResource)
        .with_resource(resources::DerivationResource)
        .with_resource(resources::ClosureResource)
        .with_resource(resources::FlakeShowResource)
        .with_resource(resources::FlakeMetadataResource)
        .with_resource(resources::StorePathInfoResource)
        .with_resource(resources::StoreLsResource)
        .with_resource(resources::StoreCatResource)
        .with_resource(resources::LogResource)
        .with_resource(resources::HashPathResource)
        .with_resource(resources::HashFileResource)
        .with_resource(resources::FhSearchResource)
        .with_resource(resources::FhListFlakesResource)
        .with_resource(resources::FhListReleasesResource)
        .with_resource(resources::FhListVersionsResource)
        .with_resource(resources::FhResolveResource)
        .with_resource(resources::FhStatusResource)
        .with_resource(resources::CachixStatusResource)
        .with_resource(resources::NilDiagnosticsResource)
        .with_resource(resources::NilHoverResource)
        .with_resource(resources::NilDefinitionResource)
        .build();

    run_stdio_server(server).await?;
    Ok(())
}
