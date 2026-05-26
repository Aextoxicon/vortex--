mod test_utils;

use test_utils::TestFixture;
use vortex__::jwt;

async fn setup_jwt_service() -> (TestFixture, jwt::JwtService) {
    let fixture = TestFixture::new().await;
    
    let jwt_service = jwt::JwtService::new(
        fixture.pool.clone(),
        "test-secret-key-min-32-chars-long!!",
        "test-issuer",
        60,
    );
    
    (fixture, jwt_service)
}

#[tokio::test]
async fn test_generate_and_validate_token() {
    let (_fixture, jwt_service) = setup_jwt_service().await;
    
    let user_id = 1;
    let public_id = "test_public_id";
    let username = "testuser";
    
    let token = jwt_service.generate_token(user_id, public_id, username).unwrap();
    assert!(!token.is_empty());
    
    let claims = jwt_service.validate_token(&token).unwrap();
    assert_eq!(claims.user_id, user_id);
    assert_eq!(claims.public_id, public_id);
    assert_eq!(claims.username, username);
}

#[tokio::test]
async fn test_validate_invalid_token() {
    let (_fixture, jwt_service) = setup_jwt_service().await;
    
    let result = jwt_service.validate_token("invalid_token");
    assert!(result.is_err());
}

#[tokio::test]
async fn test_blacklist_token() {
    let (_fixture, jwt_service) = setup_jwt_service().await;
    
    let user_id = 1;
    let public_id = "test_public_id";
    let username = "testuser";
    
    let token = jwt_service.generate_token(user_id, public_id, username).unwrap();
    let claims = jwt_service.validate_token(&token).unwrap();
    
    jwt_service.blacklist_token(&claims.jti, claims.exp).await.unwrap();
    
    assert!(jwt_service.is_blacklisted(&claims.jti));
    
    let result = jwt_service.validate_token(&token);
    assert!(result.is_err());
}

#[tokio::test]
async fn test_token_expiration() {
    let fixture = TestFixture::new().await;
    
    let jwt_service = jwt::JwtService::new(
        fixture.pool.clone(),
        "test-secret-key-min-32-chars-long!!",
        "test-issuer",
        -60,
    );
    
    let user_id = 1;
    let public_id = "test_public_id";
    let username = "testuser";
    
    let token = jwt_service.generate_token(user_id, public_id, username).unwrap();
    
    let result = jwt_service.validate_token(&token);
    assert!(result.is_err());
}
