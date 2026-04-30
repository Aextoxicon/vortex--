using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;

namespace Vortex.Entities;

[Table("user_devices")]
public class UserDevice
{
    [Key]
    [DatabaseGenerated(DatabaseGeneratedOption.Identity)]
    [Column("id")]
    public long Id { get; set; }

    [Required]
    [Column("user_id")]
    public long UserId { get; set; }

    [Required]
    [Column("device_token")]
    [MaxLength(1000)]
    public string DeviceToken { get; set; } = string.Empty;

    [Column("device_name")]
    [MaxLength(255)]
    public string? DeviceName { get; set; }

    [Column("device_type")]
    [MaxLength(50)]
    public string? DeviceType { get; set; }

    [Column("ip_address")]
    [MaxLength(45)]
    public string? IpAddress { get; set; }

    [Column("last_active_at")]
    public DateTime LastActiveAt { get; set; }

    [Column("is_active")]
    public bool IsActive { get; set; } = true;

    [Column("inserted_at")]
    public DateTime InsertedAt { get; set; } = DateTime.UtcNow;

    [Column("updated_at")]
    public DateTime UpdatedAt { get; set; } = DateTime.UtcNow;

    [ForeignKey(nameof(UserId))]
    public User? User { get; set; }
}
