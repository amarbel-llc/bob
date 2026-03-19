use async_trait::async_trait;
use mcp_server::resources::{Resource, ResourceContent, ResourceError};
use mcp_server::server::Context;

use super::parse_chix_uri;

pub struct FlakeShowResource;

#[async_trait]
impl Resource for FlakeShowResource {
    fn uri_template(&self) -> &str {
        "chix://flake/show?flake_ref={ref}"
    }

    fn name(&self) -> &str {
        "Flake Show"
    }

    fn description(&self) -> &str {
        "Show flake outputs. Query params: flake_ref (default: .)"
    }

    fn mime_type(&self) -> &str {
        "application/json"
    }

    async fn read(&self, uri: &str, _ctx: &Context) -> Result<ResourceContent, ResourceError> {
        let parsed = parse_chix_uri(uri).map_err(ResourceError::InvalidUri)?;
        let result = super::read_flake_show(&parsed)
            .await
            .map_err(ResourceError::ReadFailed)?;

        Ok(ResourceContent {
            uri: result.uri,
            mime_type: result.mime_type,
            text: result.text,
        })
    }
}
