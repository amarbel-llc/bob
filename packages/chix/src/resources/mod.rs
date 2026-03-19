mod build_log;
mod closure;
mod derivation;

// Resource trait implementation modules
mod build_log_resource;
mod cachix_status_resource;
mod closure_resource;
mod derivation_resource;
mod fh_list_flakes_resource;
mod fh_list_releases_resource;
mod fh_list_versions_resource;
mod fh_resolve_resource;
mod fh_search_resource;
mod fh_status_resource;
mod flake_metadata_resource;
mod flake_show_resource;
mod hash_file_resource;
mod hash_path_resource;
mod log_resource;
mod nil_definition_resource;
mod nil_diagnostics_resource;
mod nil_hover_resource;
mod store_cat_resource;
mod store_ls_resource;
mod store_path_info_resource;

pub use build_log::read_build_log;
pub use closure::read_closure;
pub use derivation::read_derivation;

// Resource trait implementations
pub use build_log_resource::BuildLogResource;
pub use cachix_status_resource::CachixStatusResource;
pub use closure_resource::ClosureResource;
pub use derivation_resource::DerivationResource;
pub use fh_list_flakes_resource::FhListFlakesResource;
pub use fh_list_releases_resource::FhListReleasesResource;
pub use fh_list_versions_resource::FhListVersionsResource;
pub use fh_resolve_resource::FhResolveResource;
pub use fh_search_resource::FhSearchResource;
pub use fh_status_resource::FhStatusResource;
pub use flake_metadata_resource::FlakeMetadataResource;
pub use flake_show_resource::FlakeShowResource;
pub use hash_file_resource::HashFileResource;
pub use hash_path_resource::HashPathResource;
pub use log_resource::LogResource;
pub use nil_definition_resource::NilDefinitionResource;
pub use nil_diagnostics_resource::NilDiagnosticsResource;
pub use nil_hover_resource::NilHoverResource;
pub use store_cat_resource::StoreCatResource;
pub use store_ls_resource::StoreLsResource;
pub use store_path_info_resource::StorePathInfoResource;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Resource URI format: chix://{resource_type}/{sub_type}/{path}?{params}
/// Also accepts nix:// for backwards compatibility.
/// Examples:
/// - chix://build-log/abc123-hello?offset=0&limit=1000
/// - chix://flake/show?flake_ref=.
/// - chix://store/ls//nix/store/abc123-hello
/// - chix://nil/hover//path/to/file.nix?line=10&character=5

#[derive(Debug, Serialize)]
pub struct ResourceInfo {
    pub uri: String,
    pub name: String,
    pub description: String,
    #[serde(rename = "mimeType")]
    pub mime_type: String,
}

#[derive(Debug, Serialize)]
pub struct ResourceContent {
    pub uri: String,
    #[serde(rename = "mimeType")]
    pub mime_type: String,
    pub text: String,
}

#[derive(Debug, Deserialize, Default)]
pub struct ResourceReadParams {
    pub uri: String,
}

#[derive(Debug, Clone)]
pub struct ParsedUri {
    pub resource_type: String,
    pub sub_type: Option<String>,
    pub path: String,
    pub params: HashMap<String, String>,
}

pub fn parse_chix_uri(uri: &str) -> Result<ParsedUri, String> {
    // Accept both chix:// and nix:// for backwards compatibility
    let rest = if uri.starts_with("chix://") {
        &uri[7..]
    } else if uri.starts_with("nix://") {
        &uri[6..]
    } else {
        return Err(format!(
            "Invalid URI scheme, expected chix:// or nix://: {}",
            uri
        ));
    };

    // Split path and query params
    let (path_part, query_part) = if let Some(idx) = rest.find('?') {
        (&rest[..idx], Some(&rest[idx + 1..]))
    } else {
        (rest, None)
    };

    // Split into segments: resource_type / sub_type / remaining_path
    let (resource_type, sub_type, path) = if let Some(idx) = path_part.find('/') {
        let rt = &path_part[..idx];
        let after_rt = &path_part[idx + 1..];

        // Check if there's a second segment (sub_type)
        // Empty segment means double-slash (absolute path), not a sub_type
        if let Some(idx2) = after_rt.find('/') {
            let st = &after_rt[..idx2];
            if st.is_empty() {
                // Double slash: chix://closure//nix/store/...
                let remaining = &after_rt[idx2..]; // keep leading /
                (rt, None, remaining.to_string())
            } else {
                let remaining = &after_rt[idx2 + 1..];
                (rt, Some(st.to_string()), remaining.to_string())
            }
        } else {
            // Could be sub_type with no remaining path, or just a path
            // Heuristic: known sub_types are not store paths
            let known_sub_types = [
                "show",
                "metadata",
                "path-info",
                "ls",
                "cat",
                "path",
                "file",
                "search",
                "flakes",
                "releases",
                "versions",
                "resolve",
                "status",
                "diagnostics",
                "hover",
                "definition",
            ];
            if known_sub_types.contains(&after_rt) {
                (rt, Some(after_rt.to_string()), String::new())
            } else {
                (rt, None, after_rt.to_string())
            }
        }
    } else {
        return Err(format!("Invalid URI format, expected type/path: {}", uri));
    };

    // Parse query params
    let mut params = HashMap::new();
    if let Some(query) = query_part {
        for pair in query.split('&') {
            if let Some(idx) = pair.find('=') {
                let key = &pair[..idx];
                let value = &pair[idx + 1..];
                params.insert(key.to_string(), value.to_string());
            }
        }
    }

    Ok(ParsedUri {
        resource_type: resource_type.to_string(),
        sub_type,
        path,
        params,
    })
}

/// List available resource templates
pub fn list_resources() -> Vec<ResourceInfo> {
    vec![
        ResourceInfo {
            uri: "chix://build-log/{store-path}".to_string(),
            name: "Build Log".to_string(),
            description:
                "Access build logs for a store path with pagination. Query params: offset, limit"
                    .to_string(),
            mime_type: "text/plain".to_string(),
        },
        ResourceInfo {
            uri: "chix://derivation/{drv-path}".to_string(),
            name: "Derivation".to_string(),
            description: "Access derivation data with optional summary mode. Query params: summary (true/false), offset, limit".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://closure/{store-path}".to_string(),
            name: "Store Closure".to_string(),
            description:
                "Access closure information for a store path. Query params: offset, limit"
                    .to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://flake/show?flake_ref={ref}".to_string(),
            name: "Flake Show".to_string(),
            description: "Show flake outputs. Query params: flake_ref (default: .)".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://flake/metadata?flake_ref={ref}".to_string(),
            name: "Flake Metadata".to_string(),
            description: "Show flake metadata. Query params: flake_ref (default: .)".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://store/path-info/{path}".to_string(),
            name: "Store Path Info".to_string(),
            description: "Get store path information including size and references".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://store/ls/{path}".to_string(),
            name: "Store Ls".to_string(),
            description: "List contents of a store path".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://store/cat/{path}".to_string(),
            name: "Store Cat".to_string(),
            description: "Read contents of a file in the nix store".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://log/{store-path}".to_string(),
            name: "Log".to_string(),
            description: "Get build log for a derivation or store path".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://hash/path/{path}".to_string(),
            name: "Hash Path".to_string(),
            description: "Compute hash of a path. Query params: hash_type".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://hash/file/{path}".to_string(),
            name: "Hash File".to_string(),
            description: "Compute hash of a file. Query params: hash_type".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://flakehub/search?query={query}".to_string(),
            name: "FlakeHub Search".to_string(),
            description: "Search FlakeHub for flakes. Query params: query, max_results".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://flakehub/flakes".to_string(),
            name: "FlakeHub List Flakes".to_string(),
            description: "List flakes on FlakeHub. Query params: limit, offset".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://flakehub/releases/{flake}".to_string(),
            name: "FlakeHub List Releases".to_string(),
            description: "List releases for a flake on FlakeHub".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://flakehub/versions/{flake}".to_string(),
            name: "FlakeHub List Versions".to_string(),
            description:
                "List versions for a flake on FlakeHub. Query params: version_constraint"
                    .to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://flakehub/resolve/{flake_ref}".to_string(),
            name: "FlakeHub Resolve".to_string(),
            description: "Resolve a FlakeHub flake reference to a specific version".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://flakehub/status".to_string(),
            name: "FlakeHub Status".to_string(),
            description: "Get FlakeHub cache status".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://cachix/status".to_string(),
            name: "Cachix Status".to_string(),
            description: "Get Cachix cache status".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://nil/diagnostics/{file}".to_string(),
            name: "Nil Diagnostics".to_string(),
            description: "Get Nix language diagnostics for a file. Query params: offset, limit"
                .to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://nil/hover/{file}?line={line}&character={character}".to_string(),
            name: "Nil Hover".to_string(),
            description: "Get hover information at a position in a Nix file".to_string(),
            mime_type: "application/json".to_string(),
        },
        ResourceInfo {
            uri: "chix://nil/definition/{file}?line={line}&character={character}".to_string(),
            name: "Nil Definition".to_string(),
            description: "Go to definition at a position in a Nix file".to_string(),
            mime_type: "application/json".to_string(),
        },
    ]
}

/// Read a resource by URI
pub async fn read_resource(uri: &str) -> Result<ResourceContent, String> {
    let parsed = parse_chix_uri(uri)?;

    match (
        parsed.resource_type.as_str(),
        parsed.sub_type.as_deref(),
    ) {
        ("build-log", None) => read_build_log(&parsed).await,
        ("derivation", None) => read_derivation(&parsed).await,
        ("closure", None) => read_closure(&parsed).await,
        ("flake", Some("show")) => read_flake_show(&parsed).await,
        ("flake", Some("metadata")) => read_flake_metadata(&parsed).await,
        ("store", Some("path-info")) => read_store_path_info(&parsed).await,
        ("store", Some("ls")) => read_store_ls(&parsed).await,
        ("store", Some("cat")) => read_store_cat(&parsed).await,
        ("log", None) => read_log(&parsed).await,
        ("hash", Some("path")) => read_hash_path(&parsed).await,
        ("hash", Some("file")) => read_hash_file(&parsed).await,
        ("flakehub", Some("search")) => read_fh_search(&parsed).await,
        ("flakehub", Some("flakes")) => read_fh_list_flakes(&parsed).await,
        ("flakehub", Some("releases")) => read_fh_list_releases(&parsed).await,
        ("flakehub", Some("versions")) => read_fh_list_versions(&parsed).await,
        ("flakehub", Some("resolve")) => read_fh_resolve(&parsed).await,
        ("flakehub", Some("status")) => read_fh_status(&parsed).await,
        ("cachix", Some("status")) => read_cachix_status(&parsed).await,
        ("nil", Some("diagnostics")) => read_nil_diagnostics(&parsed).await,
        ("nil", Some("hover")) => read_nil_hover(&parsed).await,
        ("nil", Some("definition")) => read_nil_definition(&parsed).await,
        _ => Err(format!("Unknown resource type: {}", uri)),
    }
}

// Read functions for new resources

async fn read_flake_show(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let params = crate::tools::NixFlakeShowParams {
        flake_ref: parsed.params.get("flake_ref").cloned(),
        all_systems: parsed
            .params
            .get("all_systems")
            .map(|s| s == "true"),
        flake_dir: parsed.params.get("flake_dir").cloned(),
        max_bytes: parsed
            .params
            .get("max_bytes")
            .and_then(|s| s.parse().ok()),
        head: parsed.params.get("head").and_then(|s| s.parse().ok()),
        tail: parsed.params.get("tail").and_then(|s| s.parse().ok()),
    };
    let result = crate::tools::nix_flake_show(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!("chix://flake/show?flake_ref={}", parsed.params.get("flake_ref").map(|s| s.as_str()).unwrap_or(".")),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_flake_metadata(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let params = crate::tools::NixFlakeMetadataParams {
        flake_ref: parsed.params.get("flake_ref").cloned(),
        flake_dir: parsed.params.get("flake_dir").cloned(),
        max_bytes: parsed
            .params
            .get("max_bytes")
            .and_then(|s| s.parse().ok()),
        head: parsed.params.get("head").and_then(|s| s.parse().ok()),
        tail: parsed.params.get("tail").and_then(|s| s.parse().ok()),
    };
    let result = crate::tools::nix_flake_metadata(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!("chix://flake/metadata?flake_ref={}", parsed.params.get("flake_ref").map(|s| s.as_str()).unwrap_or(".")),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_store_path_info(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let path = if parsed.path.starts_with("/nix/store/") {
        parsed.path.clone()
    } else {
        format!("/nix/store/{}", parsed.path)
    };
    let params = crate::tools::NixStorePathInfoParams {
        path: path.clone(),
        closure: parsed.params.get("closure").map(|s| s == "true"),
        derivation: parsed.params.get("derivation").map(|s| s == "true"),
        closure_limit: parsed
            .params
            .get("closure_limit")
            .and_then(|s| s.parse().ok()),
        closure_offset: parsed
            .params
            .get("closure_offset")
            .and_then(|s| s.parse().ok()),
    };
    let result = crate::tools::nix_store_path_info(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!("chix://store/path-info/{}", parsed.path),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_store_ls(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let path = if parsed.path.starts_with("/nix/store/") {
        parsed.path.clone()
    } else {
        format!("/nix/store/{}", parsed.path)
    };
    let params = crate::tools::NixStoreLsParams {
        path: path.clone(),
        long: parsed.params.get("long").map(|s| s == "true"),
        offset: parsed.params.get("offset").and_then(|s| s.parse().ok()),
        limit: parsed.params.get("limit").and_then(|s| s.parse().ok()),
    };
    let result = crate::tools::nix_store_ls(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!("chix://store/ls/{}", parsed.path),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_store_cat(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let path = if parsed.path.starts_with("/nix/store/") {
        parsed.path.clone()
    } else {
        format!("/nix/store/{}", parsed.path)
    };
    let params = crate::tools::NixStoreCatParams {
        path: path.clone(),
        offset: parsed.params.get("offset").and_then(|s| s.parse().ok()),
        limit: parsed.params.get("limit").and_then(|s| s.parse().ok()),
    };
    let result = crate::tools::nix_store_cat(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!("chix://store/cat/{}", parsed.path),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_log(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let installable = if parsed.path.starts_with("/nix/store/") {
        parsed.path.clone()
    } else if parsed.path.is_empty() {
        parsed
            .params
            .get("installable")
            .cloned()
            .unwrap_or_else(|| ".".to_string())
    } else {
        parsed.path.clone()
    };
    let params = crate::tools::NixLogParams {
        installable: installable.clone(),
        head: parsed.params.get("head").and_then(|s| s.parse().ok()),
        tail: parsed.params.get("tail").and_then(|s| s.parse().ok()),
        max_bytes: parsed
            .params
            .get("max_bytes")
            .and_then(|s| s.parse().ok()),
    };
    let result = crate::tools::nix_log(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!("chix://log/{}", parsed.path),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_hash_path(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let path = if parsed.path.is_empty() {
        parsed
            .params
            .get("path")
            .cloned()
            .ok_or("Missing path parameter")?
    } else {
        parsed.path.clone()
    };
    let params = crate::tools::NixHashPathParams {
        path,
        hash_type: parsed.params.get("hash_type").cloned(),
        base32: parsed.params.get("base32").map(|s| s == "true"),
        sri: parsed.params.get("sri").map(|s| s == "true"),
    };
    let result = crate::tools::nix_hash_path(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!("chix://hash/path/{}", parsed.path),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_hash_file(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let path = if parsed.path.is_empty() {
        parsed
            .params
            .get("path")
            .cloned()
            .ok_or("Missing path parameter")?
    } else {
        parsed.path.clone()
    };
    let params = crate::tools::NixHashFileParams {
        path,
        hash_type: parsed.params.get("hash_type").cloned(),
        base32: parsed.params.get("base32").map(|s| s == "true"),
        sri: parsed.params.get("sri").map(|s| s == "true"),
    };
    let result = crate::tools::nix_hash_file(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!("chix://hash/file/{}", parsed.path),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_fh_search(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let query = parsed
        .params
        .get("query")
        .cloned()
        .ok_or("Missing query parameter")?;
    let params = crate::tools::FhSearchParams {
        query,
        max_results: parsed
            .params
            .get("max_results")
            .and_then(|s| s.parse().ok()),
        offset: parsed.params.get("offset").and_then(|s| s.parse().ok()),
        limit: parsed.params.get("limit").and_then(|s| s.parse().ok()),
    };
    let result = crate::tools::fh_search(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!(
            "chix://flakehub/search?query={}",
            parsed.params.get("query").map(|s| s.as_str()).unwrap_or("")
        ),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_fh_list_flakes(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let params = crate::tools::FhListFlakesParams {
        limit: parsed.params.get("limit").and_then(|s| s.parse().ok()),
        offset: parsed.params.get("offset").and_then(|s| s.parse().ok()),
    };
    let result = crate::tools::fh_list_flakes(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: "chix://flakehub/flakes".to_string(),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_fh_list_releases(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let flake = if parsed.path.is_empty() {
        parsed
            .params
            .get("flake")
            .cloned()
            .ok_or("Missing flake parameter")?
    } else {
        parsed.path.clone()
    };
    let params = crate::tools::FhListReleasesParams {
        flake: flake.clone(),
        limit: parsed.params.get("limit").and_then(|s| s.parse().ok()),
        offset: parsed.params.get("offset").and_then(|s| s.parse().ok()),
    };
    let result = crate::tools::fh_list_releases(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!("chix://flakehub/releases/{}", flake),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_fh_list_versions(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let flake = if parsed.path.is_empty() {
        parsed
            .params
            .get("flake")
            .cloned()
            .ok_or("Missing flake parameter")?
    } else {
        parsed.path.clone()
    };
    let version_constraint = parsed
        .params
        .get("version_constraint")
        .cloned()
        .unwrap_or_else(|| "*".to_string());
    let params = crate::tools::FhListVersionsParams {
        flake: flake.clone(),
        version_constraint,
        limit: parsed.params.get("limit").and_then(|s| s.parse().ok()),
        offset: parsed.params.get("offset").and_then(|s| s.parse().ok()),
    };
    let result = crate::tools::fh_list_versions(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!("chix://flakehub/versions/{}", flake),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_fh_resolve(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let flake_ref = if parsed.path.is_empty() {
        parsed
            .params
            .get("flake_ref")
            .cloned()
            .ok_or("Missing flake_ref parameter")?
    } else {
        parsed.path.clone()
    };
    let params = crate::tools::FhResolveParams {
        flake_ref: flake_ref.clone(),
    };
    let result = crate::tools::fh_resolve(params)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!("chix://flakehub/resolve/{}", flake_ref),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_fh_status(_parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let result = crate::tools::fh_status()
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: "chix://flakehub/status".to_string(),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_cachix_status(_parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let result = crate::tools::cachix_status()
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: "chix://cachix/status".to_string(),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_nil_diagnostics(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let file_path = if parsed.path.is_empty() {
        parsed
            .params
            .get("file")
            .cloned()
            .ok_or("Missing file parameter")?
    } else if parsed.path.starts_with('/') {
        parsed.path.clone()
    } else {
        format!("/{}", parsed.path)
    };
    let offset = parsed.params.get("offset").and_then(|s| s.parse().ok());
    let limit = parsed.params.get("limit").and_then(|s| s.parse().ok());
    let result = crate::tools::nil_diagnostics(file_path.clone(), offset, limit)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!("chix://nil/diagnostics/{}", parsed.path),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_nil_hover(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let file_path = if parsed.path.is_empty() {
        parsed
            .params
            .get("file")
            .cloned()
            .ok_or("Missing file parameter")?
    } else if parsed.path.starts_with('/') {
        parsed.path.clone()
    } else {
        format!("/{}", parsed.path)
    };
    let line: u32 = parsed
        .params
        .get("line")
        .and_then(|s| s.parse().ok())
        .ok_or("Missing or invalid line parameter")?;
    let character: u32 = parsed
        .params
        .get("character")
        .and_then(|s| s.parse().ok())
        .ok_or("Missing or invalid character parameter")?;
    let result = crate::tools::nil_hover(file_path.clone(), line, character)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!(
            "chix://nil/hover/{}?line={}&character={}",
            parsed.path, line, character
        ),
        mime_type: "application/json".to_string(),
        text,
    })
}

async fn read_nil_definition(parsed: &ParsedUri) -> Result<ResourceContent, String> {
    let file_path = if parsed.path.is_empty() {
        parsed
            .params
            .get("file")
            .cloned()
            .ok_or("Missing file parameter")?
    } else if parsed.path.starts_with('/') {
        parsed.path.clone()
    } else {
        format!("/{}", parsed.path)
    };
    let line: u32 = parsed
        .params
        .get("line")
        .and_then(|s| s.parse().ok())
        .ok_or("Missing or invalid line parameter")?;
    let character: u32 = parsed
        .params
        .get("character")
        .and_then(|s| s.parse().ok())
        .ok_or("Missing or invalid character parameter")?;
    let result = crate::tools::nil_definition(file_path.clone(), line, character)
        .await
        .map_err(|e| e.to_string())?;
    let text = serde_json::to_string_pretty(&result).map_err(|e| e.to_string())?;
    Ok(ResourceContent {
        uri: format!(
            "chix://nil/definition/{}?line={}&character={}",
            parsed.path, line, character
        ),
        mime_type: "application/json".to_string(),
        text,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_chix_uri_simple() {
        let uri = "chix://build-log/abc123-hello";
        let parsed = parse_chix_uri(uri).unwrap();
        assert_eq!(parsed.resource_type, "build-log");
        assert_eq!(parsed.sub_type, None);
        assert_eq!(parsed.path, "abc123-hello");
        assert!(parsed.params.is_empty());
    }

    #[test]
    fn test_parse_chix_uri_backwards_compat_nix() {
        let uri = "nix://build-log/abc123-hello";
        let parsed = parse_chix_uri(uri).unwrap();
        assert_eq!(parsed.resource_type, "build-log");
        assert_eq!(parsed.sub_type, None);
        assert_eq!(parsed.path, "abc123-hello");
    }

    #[test]
    fn test_parse_chix_uri_with_params() {
        let uri = "chix://derivation/abc123.drv?summary=true&offset=10";
        let parsed = parse_chix_uri(uri).unwrap();
        assert_eq!(parsed.resource_type, "derivation");
        assert_eq!(parsed.sub_type, None);
        assert_eq!(parsed.path, "abc123.drv");
        assert_eq!(parsed.params.get("summary"), Some(&"true".to_string()));
        assert_eq!(parsed.params.get("offset"), Some(&"10".to_string()));
    }

    #[test]
    fn test_parse_chix_uri_full_store_path() {
        let uri = "chix://closure//nix/store/abc123-hello?limit=50";
        let parsed = parse_chix_uri(uri).unwrap();
        assert_eq!(parsed.resource_type, "closure");
        assert_eq!(parsed.sub_type, None);
        assert_eq!(parsed.path, "/nix/store/abc123-hello");
        assert_eq!(parsed.params.get("limit"), Some(&"50".to_string()));
    }

    #[test]
    fn test_parse_chix_uri_invalid_scheme() {
        let uri = "http://example.com";
        let result = parse_chix_uri(uri);
        assert!(result.is_err());
    }

    #[test]
    fn test_parse_chix_uri_flake_show() {
        let uri = "chix://flake/show?flake_ref=.";
        let parsed = parse_chix_uri(uri).unwrap();
        assert_eq!(parsed.resource_type, "flake");
        assert_eq!(parsed.sub_type, Some("show".to_string()));
        assert_eq!(parsed.path, "");
        assert_eq!(parsed.params.get("flake_ref"), Some(&".".to_string()));
    }

    #[test]
    fn test_parse_chix_uri_store_ls_with_store_path() {
        let uri = "chix://store/ls//nix/store/abc123-hello";
        let parsed = parse_chix_uri(uri).unwrap();
        assert_eq!(parsed.resource_type, "store");
        assert_eq!(parsed.sub_type, Some("ls".to_string()));
        assert_eq!(parsed.path, "/nix/store/abc123-hello");
    }

    #[test]
    fn test_parse_chix_uri_flakehub_releases() {
        let uri = "chix://flakehub/releases/NixOS/nixpkgs";
        let parsed = parse_chix_uri(uri).unwrap();
        assert_eq!(parsed.resource_type, "flakehub");
        assert_eq!(parsed.sub_type, Some("releases".to_string()));
        assert_eq!(parsed.path, "NixOS/nixpkgs");
    }

    #[test]
    fn test_parse_chix_uri_nil_diagnostics() {
        let uri = "chix://nil/diagnostics//path/to/file.nix";
        let parsed = parse_chix_uri(uri).unwrap();
        assert_eq!(parsed.resource_type, "nil");
        assert_eq!(parsed.sub_type, Some("diagnostics".to_string()));
        assert_eq!(parsed.path, "/path/to/file.nix");
    }

    #[test]
    fn test_parse_chix_uri_nil_hover() {
        let uri = "chix://nil/hover//path/to/file.nix?line=10&character=5";
        let parsed = parse_chix_uri(uri).unwrap();
        assert_eq!(parsed.resource_type, "nil");
        assert_eq!(parsed.sub_type, Some("hover".to_string()));
        assert_eq!(parsed.path, "/path/to/file.nix");
        assert_eq!(parsed.params.get("line"), Some(&"10".to_string()));
        assert_eq!(parsed.params.get("character"), Some(&"5".to_string()));
    }

    #[test]
    fn test_parse_chix_uri_flakehub_status() {
        let uri = "chix://flakehub/status";
        let parsed = parse_chix_uri(uri).unwrap();
        assert_eq!(parsed.resource_type, "flakehub");
        assert_eq!(parsed.sub_type, Some("status".to_string()));
        assert_eq!(parsed.path, "");
    }
}
