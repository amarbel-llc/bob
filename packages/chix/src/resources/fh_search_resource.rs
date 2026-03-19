use async_trait::async_trait;
use mcp_server::resources::{Resource, ResourceContent, ResourceError};
use mcp_server::server::Context;

use super::parse_chix_uri;

pub struct FhSearchResource;

#[async_trait]
impl Resource for FhSearchResource {
    fn uri_template(&self) -> &str {
        "chix://flakehub/search?query={query}"
    }

    fn name(&self) -> &str {
        "FlakeHub Search"
    }

    fn description(&self) -> &str {
        "Search FlakeHub for flakes. Query params: query, max_results"
    }

    fn mime_type(&self) -> &str {
        "application/json"
    }

    async fn read(&self, uri: &str, _ctx: &Context) -> Result<ResourceContent, ResourceError> {
        let parsed = parse_chix_uri(uri).map_err(ResourceError::InvalidUri)?;
        let result = super::read_fh_search(&parsed)
            .await
            .map_err(ResourceError::ReadFailed)?;

        Ok(ResourceContent {
            uri: result.uri,
            mime_type: result.mime_type,
            text: result.text,
        })
    }
}
