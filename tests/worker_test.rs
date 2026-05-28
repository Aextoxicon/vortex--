mod test_utils;

use vortex__::worker::{
    calculate_days_to_sunday, calculate_next_monday_delay, message_table_name_by_date,
};

#[test]
fn test_message_table_name_by_date() {
    let date = chrono::NaiveDate::from_ymd_opt(2026, 1, 1)
        .unwrap()
        .and_hms_opt(0, 0, 0)
        .unwrap()
        .and_utc();
    assert_eq!(message_table_name_by_date(date), "messages_20260101");

    let date = chrono::NaiveDate::from_ymd_opt(2026, 12, 31)
        .unwrap()
        .and_hms_opt(0, 0, 0)
        .unwrap()
        .and_utc();
    assert_eq!(message_table_name_by_date(date), "messages_20261231");

    let date = chrono::NaiveDate::from_ymd_opt(2026, 6, 15)
        .unwrap()
        .and_hms_opt(0, 0, 0)
        .unwrap()
        .and_utc();
    assert_eq!(message_table_name_by_date(date), "messages_20260615");
}

#[test]
fn test_calculate_next_monday_delay() {
    let delay = calculate_next_monday_delay();
    assert!(delay > chrono::TimeDelta::zero());
    assert!(delay < chrono::TimeDelta::days(7));
}

#[test]
fn test_calculate_days_to_sunday() {
    let sunday = chrono::NaiveDate::from_ymd_opt(2026, 5, 10).unwrap();
    assert_eq!(calculate_days_to_sunday(sunday), 0);

    let monday = chrono::NaiveDate::from_ymd_opt(2026, 5, 11).unwrap();
    assert_eq!(calculate_days_to_sunday(monday), 6);

    let saturday = chrono::NaiveDate::from_ymd_opt(2026, 5, 16).unwrap();
    assert_eq!(calculate_days_to_sunday(saturday), 1);
}
