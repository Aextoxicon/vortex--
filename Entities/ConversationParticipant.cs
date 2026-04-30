using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;

namespace Vortex.Entities;

[Table("conversation_participants")]
public class ConversationParticipant
{
    [Column("conv_id")]
    [MaxLength(255)]
    public string ConvId { get; set; } = string.Empty;

    [Column("user_id")]
    public long UserId { get; set; }

    [Column("join_ts")]
    public long JoinTs { get; set; }
}
