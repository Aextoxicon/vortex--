use vortex--::account::{validate_password, validate_username, validate_email};
use vortex--::groups::validate_group_name;

#[test]
fn test_validate_password() {
    assert!(validate_password("Test1234!"));
    assert!(!validate_password("Test1!"));
    assert!(!validate_password("Test1234!Test1234!Test1234!Test1234!Test1234!Test1234!Test1234!Test1234!Test1234!Test1234!Test1234!Test1234!Test1234!Test1234!Test"));
    assert!(!validate_password("test1234!"));
    assert!(!validate_password("TEST1234!"));
    assert!(!validate_password("TestTest!"));
    assert!(!validate_password("Test1234"));
    assert!(!validate_password(""));
    assert!(!validate_password("        "));
}

#[test]
fn test_validate_username() {
    assert!(validate_username("testuser"));
    assert!(validate_username("test_user"));
    assert!(validate_username("test123"));
    assert!(!validate_username("ab"));
    assert!(!validate_username("thisusernameiswaytoolongandexceedslimit"));
    assert!(!validate_username("test@user"));
    assert!(!validate_username(""));
    assert!(!validate_username("test user"));
}

#[test]
fn test_validate_email() {
    assert!(validate_email("test@example.com"));
    assert!(validate_email("test@mail.example.com"));
    assert!(validate_email("test+tag@example.com"));
    assert!(!validate_email("testexample.com"));
    assert!(!validate_email("test@"));
    assert!(!validate_email("test@example"));
    assert!(validate_email(""));
    assert!(!validate_email("test@.com"));
    assert!(!validate_email("test@example.com/verylongsubdomainthatexceedsthelimit"));
}

#[test]
fn test_validate_group_name() {
    assert!(validate_group_name("Test Group"));
    assert!(validate_group_name("Test_Group"));
    assert!(validate_group_name("Test-Group"));
    assert!(validate_group_name("Group123"));
    assert!(!validate_group_name("This group name is way too long and exceeds the fifty character limit"));
    assert!(!validate_group_name(""));
    assert!(!validate_group_name("Group@123"));
}
