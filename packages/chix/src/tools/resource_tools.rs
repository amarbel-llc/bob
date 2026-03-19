use async_trait::async_trait;
use mcp_server::server::Context;
use mcp_server::tools::{Tool, ToolAnnotations, ToolError, ToolResult, ToolV1};
use serde_json::Value;

use crate::resources;

pub struct ResourceTemplatesTool;

#[async_trait]
impl Tool for ResourceTemplatesTool {
    fn name(&self) -> &str {
        "resource-templates"
    }

    fn description(&self) -> &str {
        "List available chix resource templates. Call this first to discover what resources are available, then use resource-read to access them."
    }

    fn input_schema(&self) -> Value {
        serde_json::json!({
            "type": "object",
            "properties": {}
        })
    }

    async fn execute(&self, _arguments: Value, _ctx: &Context) -> Result<ToolResult, ToolError> {
        let resource_list = resources::list_resources();

        let mut sb = String::new();
        sb.push_str(
            "Resource templates (fill in {placeholders} and pass to resource-read):\n\n",
        );

        for r in &resource_list {
            sb.push_str(&format!("- {}: {}\n  {}\n", r.name, r.uri, r.description));
        }

        sb.push_str(
            "\nStore paths start with /nix/store/. Use double-slash for absolute paths in URI path segments (e.g., chix://store/ls//nix/store/abc).",
        );

        Ok(ToolResult::text(sb))
    }
}

#[async_trait]
impl ToolV1 for ResourceTemplatesTool {
    fn title(&self) -> Option<&str> {
        Some("List Resource Templates")
    }

    fn annotations(&self) -> Option<ToolAnnotations> {
        Some(ToolAnnotations {
            title: None,
            read_only_hint: Some(true),
            destructive_hint: Some(false),
            idempotent_hint: Some(true),
            open_world_hint: Some(false),
        })
    }
}

pub struct ResourceReadTool;

#[async_trait]
impl Tool for ResourceReadTool {
    fn name(&self) -> &str {
        "resource-read"
    }

    fn description(&self) -> &str {
        "Read a chix resource by URI. This tool exists because subagents cannot access MCP resources directly. Call resource-templates to discover available URIs."
    }

    fn input_schema(&self) -> Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "uri": {
                    "type": "string",
                    "description": "Resource URI (e.g., chix://flake/show?flake_ref=., chix://store/ls//nix/store/abc)"
                }
            },
            "required": ["uri"]
        })
    }

    async fn execute(&self, arguments: Value, _ctx: &Context) -> Result<ToolResult, ToolError> {
        let uri = arguments
            .get("uri")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError::InvalidArguments("Missing 'uri' parameter".to_string()))?;

        match resources::read_resource(uri).await {
            Ok(content) => Ok(ToolResult::text(content.text)),
            Err(e) => Ok(ToolResult::error(e)),
        }
    }
}

#[async_trait]
impl ToolV1 for ResourceReadTool {
    fn title(&self) -> Option<&str> {
        Some("Read Resource")
    }

    fn annotations(&self) -> Option<ToolAnnotations> {
        Some(ToolAnnotations {
            title: None,
            read_only_hint: Some(true),
            destructive_hint: Some(false),
            idempotent_hint: Some(true),
            open_world_hint: Some(false),
        })
    }
}
