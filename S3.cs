using Amazon.S3;
using Amazon.S3.Model;
using Amazon.S3.Util;

namespace Vortex;

public class S3
{
    private readonly IAmazonS3 _s3Client;
    private readonly string _bucketName;
    private readonly string _region;
    private readonly int _expiresInSeconds;
    private readonly ILogger<S3> _logger;

    public S3(IConfiguration config, ILogger<S3> logger)
    {
        _bucketName = config["S3:Bucket"] ?? throw new InvalidOperationException("S3:Bucket not configured");
        _region = config["S3:Region"] ?? "us-east-1";
        _expiresInSeconds = int.Parse(config["S3:PresignExpiresIn"] ?? "3600");
        _logger = logger;

        var s3Config = new AmazonS3Config
        {
            RegionEndpoint = Amazon.RegionEndpoint.GetBySystemName(_region)
        };

        _s3Client = new AmazonS3Client(s3Config);
    }

    public async Task<(bool Success, Error? Error)> InitBucketAsync()
    {
        if (string.IsNullOrEmpty(_bucketName))
        {
            _logger.LogError("S3 bucket not configured. Please set S3_BUCKET environment variable");
            return (false, Error.InternalError("Bucket not configured"));
        }

        try
        {
            var exists = await AmazonS3Util.DoesS3BucketExistV2Async(_s3Client, _bucketName);

            if (exists)
            {
                _logger.LogInformation("S3 bucket {Bucket} already exists", _bucketName);
                return (true, null);
            }

            var request = new PutBucketRequest
            {
                BucketName = _bucketName,
                UseClientRegion = true
            };

            await _s3Client.PutBucketAsync(request);
            _logger.LogInformation("S3 bucket {Bucket} initialized successfully", _bucketName);
            return (true, null);
        }
        catch (AmazonS3Exception ex) when (ex.ErrorCode == "BucketAlreadyOwnedByYou")
        {
            _logger.LogInformation("S3 bucket {Bucket} already exists and is owned by you", _bucketName);
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to initialize S3 bucket");
            return (false, Error.InternalError("Failed to initialize S3 bucket"));
        }
    }

    public async Task<(string? Url, Error? Error)> GenerateUploadUrlAsync(
        string filename,
        string? contentType = null,
        int? expiresInSeconds = null)
    {
        try
        {
            var expires = expiresInSeconds ?? _expiresInSeconds;

            var request = new GetPreSignedUrlRequest
            {
                BucketName = _bucketName,
                Key = filename,
                Verb = HttpVerb.PUT,
                Expires = DateTime.UtcNow.AddSeconds(expires)
            };

            if (!string.IsNullOrEmpty(contentType))
            {
                request.Headers["Content-Type"] = contentType;
            }

            var url = _s3Client.GetPreSignedURL(request);
            return (url, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to generate upload URL for {Filename}", filename);
            return (null, Error.InternalError("Failed to generate upload URL"));
        }
    }

    public async Task<(string? Url, Error? Error)> GenerateDownloadUrlAsync(
        string filename,
        int? expiresInSeconds = null)
    {
        try
        {
            var expires = expiresInSeconds ?? _expiresInSeconds;

            var request = new GetPreSignedUrlRequest
            {
                BucketName = _bucketName,
                Key = filename,
                Verb = HttpVerb.GET,
                Expires = DateTime.UtcNow.AddSeconds(expires)
            };

            var url = _s3Client.GetPreSignedURL(request);
            return (url, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to generate download URL for {Filename}", filename);
            return (null, Error.InternalError("Failed to generate download URL"));
        }
    }

    public async Task<(bool Success, Error? Error)> DeleteObjectAsync(string filename)
    {
        try
        {
            var request = new DeleteObjectRequest
            {
                BucketName = _bucketName,
                Key = filename
            };

            await _s3Client.DeleteObjectAsync(request);
            _logger.LogInformation("S3 object {Filename} deleted successfully", filename);
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to delete S3 object {Filename}", filename);
            return (false, Error.InternalError("Failed to delete S3 object"));
        }
    }

    public async Task<bool> BucketExistsAsync()
    {
        try
        {
            return await AmazonS3Util.DoesS3BucketExistV2Async(_s3Client, _bucketName);
        }
        catch
        {
            return false;
        }
    }

    public async Task<(List<S3Object>? Objects, Error? Error)> ListObjectsAsync()
    {
        try
        {
            var request = new ListObjectsV2Request
            {
                BucketName = _bucketName
            };

            var response = await _s3Client.ListObjectsV2Async(request);
            return (response.S3Objects, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to list S3 objects");
            return (null, Error.InternalError("Failed to list S3 objects"));
        }
    }
}
