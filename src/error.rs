use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use serde::Serialize;

#[derive(Debug)]
pub struct AppError {
    pub code: String,
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
            code: code_str.to_string(),
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
        match self.code.as_str() {
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

pub const ERR_NOT_FOUND: &AppError = &AppError {
    code: "not_found",
    message: "Resource not found",
};
pub const ERR_FORBIDDEN: &AppError = &AppError {
    code: "forbidden",
    message: "Access denied",
};
pub const ERR_UNAUTHORIZED: &AppError = &AppError {
    code: "unauthorized",
    message: "Unauthorized",
};
pub const ERR_INVALID_CREDENTIALS: &AppError = &AppError {
    code: "invalid_credentials",
    message: "Invalid username or password",
};
pub const ERR_RATE_LIMIT_EXCEEDED: &AppError = &AppError {
    code: "rate_limit_exceeded",
    message: "Rate limit exceeded",
};
pub const ERR_CONFLICT: &AppError = &AppError {
    code: "conflict",
    message: "Resource conflict",
};
pub const ERR_NOT_MEMBER: &AppError = &AppError {
    code: "not_member",
    message: "Not a group member",
};
pub const ERR_NOT_FRIEND: &AppError = &AppError {
    code: "not_friend",
    message: "Not friends yet, please send friend request first",
};
pub const ERR_INVALID_INPUT: &AppError = &AppError {
    code: "invalid_input",
    message: "Invalid input",
};
pub const ERR_INVALID_TYPE: &AppError = &AppError {
    code: "invalid_type",
    message: "Invalid type",
};
pub const ERR_INVALID_TARGET_ID: &AppError = &AppError {
    code: "invalid_target_id",
    message: "Invalid target ID",
};
pub const ERR_SELF_REQUEST: &AppError = &AppError {
    code: "self_request",
    message: "Cannot request self",
};
pub const ERR_NOT_PENDING: &AppError = &AppError {
    code: "not_pending",
    message: "Status is not pending",
};
pub const ERR_ALREADY_EXISTS: &AppError = &AppError {
    code: "already_exists",
    message: "Resource already exists",
};
pub const ERR_TOKEN_GENERATION: &AppError = &AppError {
    code: "token_generation_failed",
    message: "Token generation failed",
};
pub const ERR_GET_REQUESTS_FAILED: &AppError = &AppError {
    code: "get_requests_failed",
    message: "Failed to get requests",
};
pub const ERR_INVALID_DATE_FORMAT: &AppError = &AppError {
    code: "invalid_date_format",
    message: "Invalid date format, use YYYY-MM-DD",
};
pub const ERR_INVALID_MSG_ID: &AppError = &AppError {
    code: "invalid_msg_id",
    message: "Invalid message ID",
};
pub const ERR_INTERNAL_SERVER: &AppError = &AppError {
    code: "internal_server",
    message: "Internal server error",
};
pub const ERR_RECALL_WINDOW_EXPIRED: &AppError = &AppError {
    code: "recall_window_expired",
    message: "Message recall window expired",
};
pub const ERR_WEAK_PASSWORD: &AppError = &AppError {
    code: "weak_password",
    message: "Password must be 8-128 characters and contain uppercase, lowercase, number, and special character",
};
pub const ERR_INVALID_USERNAME: &AppError = &AppError {
    code: "invalid_username",
    message: "Username must be 3-20 characters and contain only letters, numbers, or common Asian characters",
};
pub const ERR_INVALID_EMAIL: &AppError = &AppError {
    code: "invalid_email",
    message: "Invalid email format",
};
pub const ERR_INVALID_GROUP_NAME: &AppError = &AppError {
    code: "invalid_group_name",
    message: "Group name must be 1-50 characters",
};
