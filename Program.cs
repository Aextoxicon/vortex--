using Microsoft.AspNetCore.Authentication.JwtBearer;
using Microsoft.IdentityModel.Tokens;
using System.Text;
using Vortex;
using Vortex.Core;
using Vortex.Auth;
using Vortex.Database;
using Microsoft.Extensions.Diagnostics.HealthChecks;
using Microsoft.AspNetCore.Diagnostics;
using System.Threading.RateLimiting;
using Vortex.Repositories;
using Vortex.Services;

var builder = WebApplication.CreateBuilder(args);

// ==================== 基础配置 ====================
builder.Services.AddControllers();
builder.Services.AddOpenApi();

// ==================== 初始化 AppConfig ====================
AppConfig.NodeId = builder.Configuration.GetValue<int>("NodeId", 0);

// ==================== 数据库配置 ====================
var databaseType = builder.Configuration.GetValue<string>("Database:Type", "sqlite");
var connectionString = builder.Configuration.GetConnectionString("DefaultConnection");

// 注册Dapper连接工厂
if (databaseType == "postgres")
{
    builder.Services.AddSingleton<IDapperConnectionFactory>(new PostgresDapperConnectionFactory(
        builder.Configuration.GetConnectionString("PostgresConnection") ?? connectionString!));
}
else
{
    builder.Services.AddSingleton<IDapperConnectionFactory>(new SqliteDapperConnectionFactory(connectionString!));
}

// ==================== JWT 认证配置 ====================
var jwtSettings = builder.Configuration.GetSection("Jwt");

var jwtSecret = builder.Configuration["Jwt:SecretKey"];

#if DEBUG
if (string.IsNullOrEmpty(jwtSecret))
{
    Console.WriteLine("⚠️  WARNING: JWT SecretKey not configured. Using default value for development.");
    jwtSecret = "dev_jwt_secret_key_for_development_only";
}
#else
if (string.IsNullOrEmpty(jwtSecret))
{
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
}
#endif

var jwtIssuer = jwtSettings["Issuer"] ?? "vortex";
var jwtAudience = jwtSettings["Audience"] ?? "vortex";

builder.Services.AddAuthentication(JwtBearerDefaults.AuthenticationScheme)
    .AddJwtBearer(options =>
    {
        options.TokenValidationParameters = new TokenValidationParameters
        {
            ValidateIssuer = jwtSettings.GetValue<bool>("ValidateIssuer"),
            ValidateAudience = jwtSettings.GetValue<bool>("ValidateAudience"),
            ValidateLifetime = jwtSettings.GetValue<bool>("ValidateLifetime"),
            ValidateIssuerSigningKey = jwtSettings.GetValue<bool>("ValidateIssuerSigningKey"),
            ValidIssuer = jwtIssuer,
            ValidAudience = jwtAudience,
            IssuerSigningKey = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(jwtSecret))
        };

        options.Events = new JwtBearerEvents
        {
            OnChallenge = context =>
            {
                context.HandleResponse();
                context.Response.StatusCode = 401;
                context.Response.ContentType = "application/json";
                return context.Response.WriteAsJsonAsync(new ErrorResponse("Unauthorized"), AppJsonContext.Default.ErrorResponse);
            },
            OnAuthenticationFailed = context =>
            {
                context.Response.StatusCode = 401;
                context.Response.ContentType = "application/json";
                return context.Response.WriteAsJsonAsync(new ErrorResponse("Invalid token"), AppJsonContext.Default.ErrorResponse);
            }
        };
    });

builder.Services.AddAuthorization();

// ==================== 限流配置 ====================
var securitySettings = builder.Configuration.GetSection("Security");
var enableRateLimiting = securitySettings.GetValue<bool>("EnableRateLimiting", true);
var maxLoginAttemptsPerMinute = securitySettings.GetValue<int>("MaxLoginAttemptsPerMinute", 5);

if (enableRateLimiting)
{
    builder.Services.AddRateLimiter(options =>
    {
        options.GlobalLimiter = PartitionedRateLimiter.Create<HttpContext, string>(
            httpContext => RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.User.Identity?.Name ?? 
                             httpContext.Connection.RemoteIpAddress?.ToString() ?? "anonymous",
                factory: partition => new FixedWindowRateLimiterOptions
                {
                    PermitLimit = 100,
                    Window = TimeSpan.FromMinutes(1)
                }));

        options.AddPolicy("login", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Connection.RemoteIpAddress?.ToString() ?? "unknown",
                factory: partition => new FixedWindowRateLimiterOptions
                {
                    PermitLimit = maxLoginAttemptsPerMinute,
                    Window = TimeSpan.FromMinutes(1)
                }));

        options.AddPolicy("register", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Connection.RemoteIpAddress?.ToString() ?? "unknown",
                factory: partition => new FixedWindowRateLimiterOptions
                {
                    PermitLimit = 10,
                    Window = TimeSpan.FromMinutes(1)
                }));

        options.OnRejected = async (context, token) =>
        {
            context.HttpContext.Response.StatusCode = 429;
            context.HttpContext.Response.ContentType = "application/json";

            var retryAfter = context.Lease.TryGetMetadata(MetadataName.RetryAfter, out var retryAfterSeconds)
                ? retryAfterSeconds.TotalSeconds
                : 60;

            await context.HttpContext.Response.WriteAsJsonAsync(
                new RateLimitResponse("Too many requests", "Rate limit exceeded. Please try again later.", (int)retryAfter),
                AppJsonContext.Default.RateLimitResponse,
                cancellationToken: token);
        };
    });
}

// ==================== 数据库维护策略 ====================
if (databaseType == "postgres")
{
    builder.Services.AddSingleton<IDatabaseMaintenanceStrategy, PostgresMaintenanceStrategy>();
}
else
{
    builder.Services.AddSingleton<IDatabaseMaintenanceStrategy, SqliteMaintenanceStrategy>();
}

// ==================== 核心服务注册 ====================
builder.Services.AddSingleton<JwtService>();

// 注册Repository层
builder.Services.AddScoped<IUserRepository, UserRepository>();
builder.Services.AddScoped<IMessageRepository, MessageRepository>();
builder.Services.AddScoped<IGroupRepository, GroupRepository>();
builder.Services.AddScoped<IFriendRequestRepository, FriendRequestRepository>();
builder.Services.AddScoped<IOutboxMessageRepository, OutboxMessageRepository>();
builder.Services.AddScoped<IIdGeneratorStateRepository, IdGeneratorStateRepository>();

// 注册Service层
builder.Services.AddScoped<IUserService, UserService>();
builder.Services.AddScoped<IMessageService, MessageService>();
builder.Services.AddScoped<IGroupService, GroupService>();
builder.Services.AddScoped<IFriendRequestService, FriendRequestService>();
builder.Services.AddSingleton<IOutboxService, OutboxService>();
builder.Services.AddScoped<SystemNotification>();

// 分布式锁和 ID 生成器
builder.Services.AddSingleton<DbLock>();
builder.Services.AddSingleton<IdGenerator>();

var features = builder.Configuration.GetSection("Features");

if (features.GetValue<bool>("EnableIdGenerator", true))
{
    builder.Services.AddHostedService(sp => sp.GetRequiredService<IdGenerator>());
}

// 数据库维护服务
if (features.GetValue<bool>("EnableDatabaseMaintenance", true))
{
    builder.Services.AddSingleton<DatabaseMaintenance>();
    
    if (features.GetValue<bool>("EnableMaintenance", true))
        builder.Services.AddHostedService<DatabaseMaintenanceScheduler>();
}

// 消息表管理
if (features.GetValue<bool>("EnableMessageTableManager", true))
{
    builder.Services.AddHostedService<MessageTableManager>();
}

// ==================== S3 配置 ====================
var s3Enabled = builder.Configuration.GetValue<bool>("S3:Enabled");
if (s3Enabled)
{
    builder.Services.AddSingleton<S3>();
}

// ==================== CORS 配置 ====================
var corsSettings = builder.Configuration.GetSection("Cors");
if (corsSettings.GetValue<bool>("Enabled"))
{
    builder.Services.AddCors(options =>
    {
        options.AddPolicy("AllowFrontend", policy =>
        {
            var allowedOrigins = corsSettings.GetSection("AllowedOrigins").Get<string[]>() 
                ?? Array.Empty<string>();
            
            policy.WithOrigins(allowedOrigins)
                  .AllowAnyHeader()
                  .AllowAnyMethod()
                  .AllowCredentials();
        });
    });
}

// ==================== 缓存配置 ====================
var cacheSettings = builder.Configuration.GetSection("Cache");
if (cacheSettings.GetValue<bool>("Enabled"))
{
    var cacheType = cacheSettings.GetValue<string>("Type", "memory");
    
    if (cacheType == "redis" && !string.IsNullOrEmpty(cacheSettings["RedisConnection"]))
    {
        builder.Services.AddStackExchangeRedisCache(options =>
        {
            options.Configuration = cacheSettings["RedisConnection"];
        });
    }
    else
    {
        builder.Services.AddMemoryCache();
    }
}

// ==================== 健康检查 ====================
var healthCheckSettings = builder.Configuration.GetSection("HealthCheck");
if (healthCheckSettings.GetValue<bool>("Enabled"))
{
    var healthChecksBuilder = builder.Services.AddHealthChecks()
        .AddCheck("self", () => HealthCheckResult.Healthy());

    if (databaseType == "postgres")
    {
        healthChecksBuilder.AddNpgSql(builder.Configuration.GetConnectionString("PostgresConnection") ?? connectionString!);
    }
    else
    {
        healthChecksBuilder.AddSqlite(connectionString!);
    }
}

var app = builder.Build();

// ==================== 中间件配置 ====================

if (corsSettings.GetValue<bool>("Enabled"))
{
    app.UseCors("AllowFrontend");
}

if (enableRateLimiting)
{
    app.UseRateLimiter();
}

app.UseExceptionHandler(errorApp =>
{
    errorApp.Run(async context =>
    {
        context.Response.StatusCode = 500;
        context.Response.ContentType = "application/json";

        var exceptionHandlerPathFeature = 
            context.Features.Get<IExceptionHandlerPathFeature>();

        if (exceptionHandlerPathFeature?.Error is Exception ex)
        {
            var logger = context.RequestServices
                .GetRequiredService<ILogger<Program>>();
            logger.LogError(ex, "Unhandled exception occurred: {Message}", ex.Message);
        }

        await context.Response.WriteAsJsonAsync(
            new ErrorResponse("Internal server error"),
            AppJsonContext.Default.ErrorResponse);
    });
});

// 初始化数据库
using (var scope = app.Services.CreateScope())
{
    var connectionFactory = scope.ServiceProvider.GetRequiredService<IDapperConnectionFactory>();
    
    if (databaseType == "sqlite")
    {
        await SqliteOptimizer.OptimizeAsync(connectionFactory);
    }
}

if (app.Environment.IsDevelopment())
{
    app.MapOpenApi();
    app.UseDeveloperExceptionPage();
}

app.UseHttpsRedirection();
app.UseAuthentication();
app.UseMiddleware<DeviceTokenMiddleware>();
app.UseAuthorization();

if (healthCheckSettings.GetValue<bool>("Enabled"))
{
    var healthPath = healthCheckSettings.GetValue<string>("Path", "/health");
    app.MapHealthChecks(healthPath);
}

app.MapControllers();

app.Run();
