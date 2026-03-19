use async_trait::async_trait;
use mcp_server::resources::{Resource, ResourceContent, ResourceError};
use mcp_server::server::Context;

use super::parse_chix_uri;

pub struct NilHoverResource;

#[async_trait]
impl Resource for NilHoverResource {
    fn uri_template(&self) -> &str {
        "chix://nil/hover/{file}?line={line}&character={character}"
    }

    fn name(&self) -> &str {
        "Nil Hover"
    }

    fn description(&self) -> &str {
        "Get hover information at a position in a Nix file"
    }

    fn mime_type(&self) -> &str {
        "application/json"
    }

    async fn read(&self, uri: &str, _ctx: &Context) -> Result<ResourceContent, ResourceError> {
        let parsed = parse_chix_uri(uri).map_err(ResourceError::InvalidUri)?;
        let result = super::read_nil_hover(&parsed)
            .await
            .map_err(ResourceError::ReadFailed)?;

        Ok(ResourceContent {
            uri: result.uri,
            mime_type: result.mime_type,
            text: result.text,
        })
    }
}
