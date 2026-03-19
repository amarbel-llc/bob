use async_trait::async_trait;
use mcp_server::resources::{Resource, ResourceContent, ResourceError};
use mcp_server::server::Context;

use super::parse_chix_uri;

pub struct HashFileResource;

#[async_trait]
impl Resource for HashFileResource {
    fn uri_template(&self) -> &str {
        "chix://hash/file/{path}"
    }

    fn name(&self) -> &str {
        "Hash File"
    }

    fn description(&self) -> &str {
        "Compute hash of a file. Query params: hash_type"
    }

    fn mime_type(&self) -> &str {
        "application/json"
    }

    async fn read(&self, uri: &str, _ctx: &Context) -> Result<ResourceContent, ResourceError> {
        let parsed = parse_chix_uri(uri).map_err(ResourceError::InvalidUri)?;
        let result = super::read_hash_file(&parsed)
            .await
            .map_err(ResourceError::ReadFailed)?;

        Ok(ResourceContent {
            uri: result.uri,
            mime_type: result.mime_type,
            text: result.text,
        })
    }
}
