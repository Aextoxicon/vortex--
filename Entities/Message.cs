using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;

namespace Vortex.Entities;

[Table("messages")]
public class Message
{
    [Key]
    [DatabaseGenerated(DatabaseGeneratedOption.None)]
    [Column("msg_id")]
    public long MsgId { get; set; }

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
}
