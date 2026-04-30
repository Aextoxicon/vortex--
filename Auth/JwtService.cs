using System.IdentityModel.Tokens.Jwt;
using System.Security.Claims;
using System.Text;
using Microsoft.IdentityModel.Tokens;
using Vortex.Entities;

namespace Vortex.Auth;

public class JwtService
{
    private readonly IConfiguration _config;
    private readonly ILogger<JwtService> _logger;

    public JwtService(IConfiguration config, ILogger<JwtService> logger)
    {
        _config = config;
        _logger = logger;
    }

    public (string? Token, Error? Error) GenerateToken(User user, string? deviceToken = null)
    {
        try
        {
            var secretKey = GetJwtSecretKey();
            var issuer = _config["Jwt:Issuer"] ?? "vortex";
            var expiresInMinutes = _config.GetValue<int>("Jwt:ExpiresInMinutes", 10080); // 默� 7 �?
            var key = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(secretKey));
            var credentials = new SigningCredentials(key, SecurityAlgorithms.HmacSha256);

            var claims = new List<Claim>
            {
                new(JwtRegisteredClaimNames.Sub, user.Id.ToString()),
                new("public_id", user.PublicId),
                new("username", user.Username),
                new(JwtRegisteredClaimNames.Jti, Guid.NewGuid().ToString())
            };

            if (!string.IsNullOrEmpty(deviceToken))
            {
                claims.Add(new("device_token", deviceToken));
            }

            var token = new JwtSecurityToken(
                issuer: issuer,
                audience: issuer,
                claims: claims,
                expires: DateTime.UtcNow.AddMinutes(expiresInMinutes),
                signingCredentials: credentials
            );

            var tokenString = new JwtSecurityTokenHandler().WriteToken(token);
            return (tokenString, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Token generation failed");
            return (null, Error.TokenGenerationFailed());
        }
    }

    public (ClaimsPrincipal? Principal, Error? Error) ValidateToken(string token)
    {
        try
        {
            var secretKey = GetJwtSecretKey();
            var issuer = _config["Jwt:Issuer"] ?? "vortex";

            var key = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(secretKey));

            var tokenHandler = new JwtSecurityTokenHandler();
            var validationParameters = new TokenValidationParameters
            {
                ValidateIssuer = true,
                ValidateAudience = true,
                ValidateLifetime = true,
                ValidateIssuerSigningKey = true,
                ValidIssuer = issuer,
                ValidAudience = issuer,
                IssuerSigningKey = key
            };

            var principal = tokenHandler.ValidateToken(token, validationParameters, out _);
            return (principal, null);
        }
        catch (Exception ex)
        {
            _logger.LogWarning(ex, "Token validation failed");
            return (null, Error.Unauthorized("Invalid token"));
        }
    }

    public TokenValidationParameters GetValidationParameters()
    {
        var secretKey = GetJwtSecretKey();
        var issuer = _config["Jwt:Issuer"] ?? "vortex";

        var key = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(secretKey));

        return new TokenValidationParameters
        {
            ValidateIssuer = true,
            ValidateAudience = true,
            ValidateLifetime = true,
            ValidateIssuerSigningKey = true,
            ValidIssuer = issuer,
            ValidAudience = issuer,
            IssuerSigningKey = key
        };
    }

    /// <summary>
    /// 获取 JWT 密钥
    /// 从配置文件读取，Debug 模式允许使用默认值，Release 模式强制配置
    /// </summary>
    private string GetJwtSecretKey()
    {
        var secretKey = _config["Jwt:SecretKey"];

        if (!string.IsNullOrEmpty(secretKey))
        {
            return secretKey;
        }

#if DEBUG
        // Debug 模式：使用默认值（仅用于开发测试）
        _logger.LogWarning("Using default JWT secret key in Debug mode. Configure Jwt:SecretKey in appsettings.json for production.");
        return "dev_jwt_secret_key_for_development_only";
#else
        // Release 模式：配置文件未设置则抛出异常
        var errorMessage = """
❌ JWT SecretKey is required in production.

To generate a secure JWT secret key, run one of these commands:

1. Using OpenSSL (Linux/macOS):
   openssl rand -base64 32

2. Using PowerShell (Windows):
   [Convert]::ToBase64String((1..32 | ForEach-Object { [byte](Get-Random -Maximum 256) }))

3. Using .NET CLI:
   dotnet user-secrets set "Jwt:SecretKey" "$(openssl rand -base64 32)"

Then configure it in appsettings.Production.json or use environment variables:
{
  \"Jwt\": {
    \"SecretKey\": \"YOUR_GENERATED_SECRET_KEY_HERE\"
  }
}
""";
        throw new InvalidOperationException(errorMessage);
#endif
    }
}
