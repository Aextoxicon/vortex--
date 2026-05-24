mod test_utils;

use test_utils::TestFixture;
use vortex--::worker::{message_table_name_by_date, calculate_next_monday_delay, calculate_days_to_sunday};
use chrono::{Utc, Datelike};

#[test]
fn test_message_table_name_by_date() {
    let date = chrono::NaiveDate::from_ymd_opt(2026, 1, 1).unwrap();
    assert_eq!(message_table_name_by_date(date), "messages_20260101");

    let date = chrono::NaiveDate::from_ymd_opt(2026, 12, 31).unwrap();
    assert_eq!(message_table_name_by_date(date), "messages_20261231");

    let date = chrono::NaiveDate::from_ymd_opt(2026, 6, 15).unwrap();
    assert_eq!(message_table_name_by_date(date), "messages_20260615");
}

#[test]
fn test_calculate_next_monday_delay() {
    let delay = calculate_next_monday_delay();
    assert!(delay > chrono::TimeDelta::zero());
    let max_delay = chrono::TimeDelta::days(7);
    assert!(delay < max_delay);
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
