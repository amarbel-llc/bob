use async_trait::async_trait;
use mcp_server::resources::{Resource, ResourceContent, ResourceError};
use mcp_server::server::Context;

use super::parse_chix_uri;

pub struct FhListVersionsResource;

#[async_trait]
impl Resource for FhListVersionsResource {
    fn uri_template(&self) -> &str {
        "chix://flakehub/versions/{flake}"
    }

    fn name(&self) -> &str {
        "FlakeHub List Versions"
    }

    fn description(&self) -> &str {
        "List versions for a flake on FlakeHub. Query params: version_constraint"
    }

    fn mime_type(&self) -> &str {
        "application/json"
    }

    async fn read(&self, uri: &str, _ctx: &Context) -> Result<ResourceContent, ResourceError> {
        let parsed = parse_chix_uri(uri).map_err(ResourceError::InvalidUri)?;
        let result = super::read_fh_list_versions(&parsed)
            .await
            .map_err(ResourceError::ReadFailed)?;

        Ok(ResourceContent {
            uri: result.uri,
            mime_type: result.mime_type,
            text: result.text,
        })
    }
}
