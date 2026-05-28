use aws_config::BehaviorVersion;
use aws_sdk_s3::Client;
use aws_sdk_s3::config::Credentials;
use aws_sdk_s3::presigning::PresigningConfig;
use axum::Json;
use axum::extract::State;
use axum::http::StatusCode;
use axum::response::IntoResponse;
use serde::{Deserialize, Serialize};
use std::time::Duration;

use crate::error::AppError;
use crate::shared::Service;

pub struct S3Service {
    client: Client,
    presigner: Client,
    bucket: String,
}

impl Clone for S3Service {
    fn clone(&self) -> Self {
        Self {
            client: self.client.clone(),
            presigner: self.presigner.clone(),
            bucket: self.bucket.clone(),
        }
    }
}

impl S3Service {
    pub async fn new(
        bucket: &str,
        region: &str,
        endpoint: &str,
        access_key: &str,
        secret_key: &str,
    ) -> Result<Self, String> {
        let region = aws_sdk_s3::config::Region::new(region.to_string());
        let mut config_builder = aws_config::defaults(BehaviorVersion::latest()).region(region);

        if !endpoint.is_empty() {
            config_builder = config_builder.endpoint_url(endpoint);
        }

        if !access_key.is_empty() && !secret_key.is_empty() {
            let creds = Credentials::new(access_key, secret_key, None, None, "static");
            config_builder = config_builder.credentials_provider(creds);
        }

        let sdk_config = config_builder.load().await;
        let client = Client::new(&sdk_config);

        Ok(Self {
            client: client.clone(),
            presigner: client,
            bucket: bucket.to_string(),
        })
    }

    pub async fn generate_upload_url(
        &self,
        conv_id: &str,
        file_ext: &str,
    ) -> Result<(String, String), AppError> {
        let file_key = generate_file_key(conv_id, file_ext);

        let presign_config = PresigningConfig::builder()
            .expires_in(Duration::from_secs(120))
            .build()
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let presigned = self
            .presigner
            .put_object()
            .bucket(&self.bucket)
            .key(&file_key)
            .presigned(presign_config)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok((presigned.uri().to_string(), file_key))
    }

    pub async fn generate_download_url(&self, file_key: &str) -> Result<String, AppError> {
        let presign_config = PresigningConfig::builder()
            .expires_in(Duration::from_secs(604800))
            .build()
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let presigned = self
            .presigner
            .get_object()
            .bucket(&self.bucket)
            .key(file_key)
            .presigned(presign_config)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(presigned.uri().to_string())
    }

    pub async fn delete_object(&self, file_key: &str) -> Result<(), AppError> {
        self.client
            .delete_object()
            .bucket(&self.bucket)
            .key(file_key)
            .send()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(())
    }
}

#[derive(Debug, Deserialize)]
pub struct PresignRequest {
    pub operation: String,
    #[serde(default)]
    pub conv_id: String,
    #[serde(default)]
    pub file_ext: String,
    #[serde(default)]
    pub file_key: String,
}

#[derive(Debug, Serialize)]
pub struct PresignResponse {
    pub url: String,
    pub file_key: String,
    pub method: String,
    pub expires_in: u64,
}

impl Service {
    pub async fn get_presign_url(
        &self,
        user_id: i64,
        req: PresignRequest,
    ) -> Result<impl IntoResponse + use<>, AppError> {
        let s3_service = self.s3_service.as_ref().ok_or_else(|| {
            AppError::new(
                StatusCode::INTERNAL_SERVER_ERROR,
                "S3 service not configured",
            )
        })?;

        match req.operation.as_str() {
            "upload" => {
                if req.conv_id.is_empty() || req.file_ext.is_empty() {
                    return Err(AppError::bad_request("conv_id and file_ext are required"));
                }

                let has_perm = self.conv_part_store.exists(&req.conv_id, user_id).await?;

                if !has_perm {
                    if let Some(group_id) = crate::shared::extract_group_id(&req.conv_id) {
                        let is_member = self.group_mem_store.is_member(&group_id, user_id).await?;
                        if !is_member {
                            return Err(AppError::forbidden());
                        }
                    } else {
                        return Err(AppError::forbidden());
                    }
                }

                let (url, file_key) = s3_service
                    .generate_upload_url(&req.conv_id, &req.file_ext)
                    .await?;

                Ok(Json(PresignResponse {
                    url,
                    file_key,
                    method: "PUT".to_string(),
                    expires_in: 120,
                }))
            }
            "download" => {
                if req.file_key.is_empty() {
                    return Err(AppError::bad_request("file_key is required"));
                }

                let conv_id = extract_conv_id_from_key(&req.file_key)
                    .ok_or_else(|| AppError::bad_request("invalid file key format"))?;

                let has_perm = self.conv_part_store.exists(&conv_id, user_id).await?;

                if !has_perm {
                    if let Some(group_id) = crate::shared::extract_group_id(&conv_id) {
                        let is_member = self.group_mem_store.is_member(&group_id, user_id).await?;
                        if !is_member {
                            return Err(AppError::forbidden());
                        }
                    } else {
                        return Err(AppError::forbidden());
                    }
                }

                let url = s3_service.generate_download_url(&req.file_key).await?;

                Ok(Json(PresignResponse {
                    url,
                    file_key: req.file_key,
                    method: "GET".to_string(),
                    expires_in: 604800,
                }))
            }
            _ => Err(AppError::bad_request("invalid operation")),
        }
    }
}

pub async fn presign(
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    Json(req): Json<PresignRequest>,
) -> Result<impl IntoResponse + use<>, AppError> {
    state.svc.get_presign_url(user_id, req).await
}

pub fn generate_file_key(conv_id: &str, file_ext: &str) -> String {
    let id = uuid::Uuid::new_v4();
    format!("uploads/{}/{}.{}", conv_id, id, file_ext)
}

pub fn extract_conv_id_from_key(file_key: &str) -> Option<String> {
    let parts: Vec<&str> = file_key.split('/').collect();
    if parts.len() >= 3 && parts[0] == "uploads" {
        Some(parts[1].to_string())
    } else {
        None
    }
}
