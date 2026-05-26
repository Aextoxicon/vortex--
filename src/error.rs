use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use serde::Serialize;

#[derive(Debug)]
pub struct AppError {
    pub code: &'static str,
    pub message: String,
}

#[derive(Serialize)]
pub struct ErrorResponse {
    pub error: String,
}

#[derive(Serialize)]
pub struct SuccessResponse {
    pub success: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub message: Option<String>,
}

impl AppError {
    pub fn new(code: StatusCode, message: &str) -> Self {
        let code_str = match code {
            StatusCode::NOT_FOUND => "not_found",
            StatusCode::FORBIDDEN => "forbidden",
            StatusCode::UNAUTHORIZED => "unauthorized",
            StatusCode::CONFLICT => "conflict",
            StatusCode::TOO_MANY_REQUESTS => "rate_limit_exceeded",
            StatusCode::BAD_REQUEST => "invalid_input",
            _ => "internal_server",
        };
        Self {
            code: code_str,
            message: message.to_string(),
        }
    }

    pub fn bad_request(message: &str) -> Self {
        Self::new(StatusCode::BAD_REQUEST, message)
    }

    pub fn not_found(message: &str) -> Self {
        Self::new(StatusCode::NOT_FOUND, message)
    }

    pub fn forbidden() -> Self {
        Self::new(StatusCode::FORBIDDEN, "Access denied")
    }

    pub fn conflict(message: &str) -> Self {
        Self::new(StatusCode::CONFLICT, message)
    }

    pub fn status_code(&self) -> StatusCode {
        match self.code {
            "not_found" => StatusCode::NOT_FOUND,
            "forbidden" => StatusCode::FORBIDDEN,
            "unauthorized" | "invalid_credentials" => StatusCode::UNAUTHORIZED,
            "already_exists" | "conflict" | "self_request" => StatusCode::CONFLICT,
            "rate_limit_exceeded" => StatusCode::TOO_MANY_REQUESTS,
            "invalid_input"
            | "invalid_type"
            | "invalid_target_id"
            | "not_pending"
            | "not_member"
            | "invalid_date_format"
            | "invalid_msg_id"
            | "token_generation_failed"
            | "get_requests_failed"
            | "recall_window_expired"
            | "weak_password"
            | "invalid_username"
            | "invalid_email"
            | "invalid_group_name" => StatusCode::BAD_REQUEST,
            _ => StatusCode::INTERNAL_SERVER_ERROR,
        }
    }
}

impl IntoResponse for AppError {
    fn into_response(self) -> Response {
        let status = self.status_code();

        let body = ErrorResponse {
            error: self.message,
        };
        (status, axum::Json(body)).into_response()
    }
}

impl From<sqlx::Error> for AppError {
    fn from(e: sqlx::Error) -> Self {
        AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string())
    }
}

pub fn err_not_found() -> AppError {
    AppError {
        code: "not_found",
        message: "Resource not found".to_string(),
    }
}
pub fn err_forbidden() -> AppError {
    AppError {
        code: "forbidden",
        message: "Access denied".to_string(),
    }
}
pub fn err_unauthorized() -> AppError {
    AppError {
        code: "unauthorized",
        message: "Unauthorized".to_string(),
    }
}
pub fn err_invalid_credentials() -> AppError {
    AppError {
        code: "invalid_credentials",
        message: "Invalid username or password".to_string(),
    }
}
pub fn err_rate_limit_exceeded() -> AppError {
    AppError {
        code: "rate_limit_exceeded",
        message: "Rate limit exceeded".to_string(),
    }
}
pub fn err_conflict() -> AppError {
    AppError {
        code: "conflict",
        message: "Resource conflict".to_string(),
    }
}
pub fn err_not_member() -> AppError {
    AppError {
        code: "not_member",
        message: "Not a group member".to_string(),
    }
}
pub fn err_not_friend() -> AppError {
    AppError {
        code: "not_friend",
        message: "Not friends yet, please send friend request first".to_string(),
    }
}
pub fn err_invalid_input() -> AppError {
    AppError {
        code: "invalid_input",
        message: "Invalid input".to_string(),
    }
}
pub fn err_invalid_type() -> AppError {
    AppError {
        code: "invalid_type",
        message: "Invalid type".to_string(),
    }
}
pub fn err_invalid_target_id() -> AppError {
    AppError {
        code: "invalid_target_id",
        message: "Invalid target ID".to_string(),
    }
}
pub fn err_self_request() -> AppError {
    AppError {
        code: "self_request",
        message: "Cannot request self".to_string(),
    }
}
pub fn err_not_pending() -> AppError {
    AppError {
        code: "not_pending",
        message: "Status is not pending".to_string(),
    }
}
pub fn err_already_exists() -> AppError {
    AppError {
        code: "already_exists",
        message: "Resource already exists".to_string(),
    }
}
pub fn err_token_generation() -> AppError {
    AppError {
        code: "token_generation_failed",
        message: "Token generation failed".to_string(),
    }
}
pub fn err_get_requests_failed() -> AppError {
    AppError {
        code: "get_requests_failed",
        message: "Failed to get requests".to_string(),
    }
}
pub fn err_invalid_date_format() -> AppError {
    AppError {
        code: "invalid_date_format",
        message: "Invalid date format, use YYYY-MM-DD".to_string(),
    }
}
pub fn err_invalid_msg_id() -> AppError {
    AppError {
        code: "invalid_msg_id",
        message: "Invalid message ID".to_string(),
    }
}
pub fn err_internal_server() -> AppError {
    AppError {
        code: "internal_server",
        message: "Internal server error".to_string(),
    }
}
pub fn err_recall_window_expired() -> AppError {
    AppError {
        code: "recall_window_expired",
        message: "Message recall window expired".to_string(),
    }
}
pub fn err_weak_password() -> AppError {
    AppError {
        code: "weak_password",
        message: "Password must be 8-128 characters and contain uppercase, lowercase, number, and special character".to_string(),
    }
}
pub fn err_invalid_username() -> AppError {
    AppError {
        code: "invalid_username",
        message: "Username must be 3-20 characters and contain only letters, numbers, or common Asian characters".to_string(),
    }
}
pub fn err_invalid_email() -> AppError {
    AppError {
        code: "invalid_email",
        message: "Invalid email format".to_string(),
    }
}
pub fn err_invalid_group_name() -> AppError {
    AppError {
        code: "invalid_group_name",
        message: "Group name must be 1-50 characters".to_string(),
    }
}
