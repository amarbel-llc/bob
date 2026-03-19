use async_trait::async_trait;
use mcp_server::resources::{Resource, ResourceContent, ResourceError};
use mcp_server::server::Context;

use super::parse_chix_uri;

pub struct FhListFlakesResource;

#[async_trait]
impl Resource for FhListFlakesResource {
    fn uri_template(&self) -> &str {
        "chix://flakehub/flakes"
    }

    fn name(&self) -> &str {
        "FlakeHub List Flakes"
    }

    fn description(&self) -> &str {
        "List flakes on FlakeHub. Query params: limit, offset"
    }

    fn mime_type(&self) -> &str {
        "application/json"
    }

    async fn read(&self, uri: &str, _ctx: &Context) -> Result<ResourceContent, ResourceError> {
        let parsed = parse_chix_uri(uri).map_err(ResourceError::InvalidUri)?;
        let result = super::read_fh_list_flakes(&parsed)
            .await
            .map_err(ResourceError::ReadFailed)?;

        Ok(ResourceContent {
            uri: result.uri,
            mime_type: result.mime_type,
            text: result.text,
        })
    }
}
