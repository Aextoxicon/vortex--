using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;

namespace Vortex.Entities;

[Table("outbox_messages")]
public class OutboxMessage
{
    [Key]
    [Column("id")]
    public long Id { get; set; }

    [Required]
    [Column("msg_id")]
    [MaxLength(255)]
    public string MsgId { get; set; } = string.Empty;

    [Required]
    [Column("conv_id")]
    [MaxLength(255)]
    public string ConvId { get; set; } = string.Empty;

    [Required]
    [Column("from_uid")]
    public long FromUid { get; set; }

    [Required]
    [Column("content")]
    public string Content { get; set; } = string.Empty;

    [Required]
    [Column("ts")]
    public long Ts { get; set; }

    [Required]
    [Column("is_recalled")]
    public int IsRecalled { get; set; } = 0;

    [Required]
    [Column("status")]
    [MaxLength(50)]
    public string Status { get; set; } = "pending";

    [Required]
    [Column("retry_count")]
    public int RetryCount { get; set; } = 0;

    [Required]
    [Column("created_at")]
    public DateTime CreatedAt { get; set; } = DateTime.UtcNow;

    [Required]
    [Column("updated_at")]
    public DateTime UpdatedAt { get; set; } = DateTime.UtcNow;
}
