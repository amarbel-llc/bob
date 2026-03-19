use async_trait::async_trait;
use mcp_server::resources::{Resource, ResourceContent, ResourceError};
use mcp_server::server::Context;

use super::parse_chix_uri;

pub struct StoreCatResource;

#[async_trait]
impl Resource for StoreCatResource {
    fn uri_template(&self) -> &str {
        "chix://store/cat/{path}"
    }

    fn name(&self) -> &str {
        "Store Cat"
    }

    fn description(&self) -> &str {
        "Read contents of a file in the nix store"
    }

    fn mime_type(&self) -> &str {
        "application/json"
    }

    async fn read(&self, uri: &str, _ctx: &Context) -> Result<ResourceContent, ResourceError> {
        let parsed = parse_chix_uri(uri).map_err(ResourceError::InvalidUri)?;
        let result = super::read_store_cat(&parsed)
            .await
            .map_err(ResourceError::ReadFailed)?;

        Ok(ResourceContent {
            uri: result.uri,
            mime_type: result.mime_type,
            text: result.text,
        })
    }
}
