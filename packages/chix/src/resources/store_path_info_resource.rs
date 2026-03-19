use async_trait::async_trait;
use mcp_server::resources::{Resource, ResourceContent, ResourceError};
use mcp_server::server::Context;

use super::parse_chix_uri;

pub struct StorePathInfoResource;

#[async_trait]
impl Resource for StorePathInfoResource {
    fn uri_template(&self) -> &str {
        "chix://store/path-info/{path}"
    }

    fn name(&self) -> &str {
        "Store Path Info"
    }

    fn description(&self) -> &str {
        "Get store path information including size and references"
    }

    fn mime_type(&self) -> &str {
        "application/json"
    }

    async fn read(&self, uri: &str, _ctx: &Context) -> Result<ResourceContent, ResourceError> {
        let parsed = parse_chix_uri(uri).map_err(ResourceError::InvalidUri)?;
        let result = super::read_store_path_info(&parsed)
            .await
            .map_err(ResourceError::ReadFailed)?;

        Ok(ResourceContent {
            uri: result.uri,
            mime_type: result.mime_type,
            text: result.text,
        })
    }
}
