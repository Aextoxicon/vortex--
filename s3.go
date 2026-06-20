package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
)

type S3Service struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
	region    string
}

func NewS3Service(ctx context.Context, bucket, region, endpoint, accessKey, secretKey string) (*S3Service, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	if endpoint != "" {
		customResolver := aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: endpoint}, nil
			},
		)
		opts = append(opts, config.WithEndpointResolverWithOptions(customResolver))
	}

	if accessKey != "" && secretKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg)
	presigner := s3.NewPresignClient(client)

	return &S3Service{
		client:    client,
		presigner: presigner,
		bucket:    bucket,
		region:    region,
	}, nil
}

func (s *S3Service) GenerateUploadURL(ctx context.Context, convID, fileExt string) (string, string, error) {
	fileKey := GenerateFileKey(convID, fileExt)

	result, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fileKey),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = 2 * time.Minute
	})

	if err != nil {
		return "", "", fmt.Errorf("failed to generate upload URL: %w", err)
	}

	return result.URL, fileKey, nil
}

func (s *S3Service) GenerateDownloadURL(ctx context.Context, fileKey string) (string, error) {
	result, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fileKey),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = 7 * 24 * time.Hour
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate download URL: %w", err)
	}

	return result.URL, nil
}

func (s *S3Service) DeleteObject(ctx context.Context, fileKey string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fileKey),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

func GenerateFileKey(convID, fileExt string) string {
	id := GenerateNanoID(21)
	return fmt.Sprintf("uploads/%s/%s.%s", convID, id, fileExt)
}

func ExtractConvIDFromKey(fileKey string) (string, error) {
	parts := strings.Split(fileKey, "/")
	if len(parts) < 3 || parts[0] != "uploads" {
		return "", fmt.Errorf("invalid file key format")
	}
	return parts[1], nil
}

type PresignRequest struct {
	Operation string `json:"operation" binding:"required,oneof=upload download"`
	ConvID    string `json:"conv_id"`
	FileExt   string `json:"file_ext"`
	FileKey   string `json:"file_key"`
}

// GetPresignURL 获取文件预签名 URL
// @Summary      获取文件预签名 URL
// @Description  获取文件上传或下载的预签名 URL
// @Tags         files
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        request  body  PresignRequest  true  "预签名请求"
// @Success      200  {object}  map[string]interface{}  "预签名 URL"
// @Failure      400  {object}  ErrorResponse  "请求无效"
// @Failure      501  {object}  ErrorResponse  "S3 服务未配置"
// @Router       /api/files/presign [post]
func (h *Handler) GetPresignURL(c *gin.Context) {
	userID := c.GetInt64("user_id")

	if h.svc.s3Service == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "S3 service not configured"})
		return
	}

	var req PresignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, ErrInvalidInput)
		return
	}

	switch req.Operation {
	case "upload":
		if req.ConvID == "" || req.FileExt == "" {
			handleError(c, ErrInvalidInput)
			return
		}

		if err := h.checkConvPermission(c.Request.Context(), req.ConvID, userID); err != nil {
			handleError(c, err)
			return
		}

		url, fileKey, err := h.svc.s3Service.GenerateUploadURL(c.Request.Context(), req.ConvID, req.FileExt)
		if err != nil {
			handleError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"url":        url,
			"file_key":   fileKey,
			"method":     "PUT",
			"expires_in": 120,
		})

	case "download":
		if req.FileKey == "" {
			handleError(c, ErrInvalidInput)
			return
		}

		convID, err := ExtractConvIDFromKey(req.FileKey)
		if err != nil {
			handleError(c, ErrInvalidInput)
			return
		}

		if err := h.checkConvPermission(c.Request.Context(), convID, userID); err != nil {
			handleError(c, err)
			return
		}

		url, err := h.svc.s3Service.GenerateDownloadURL(c.Request.Context(), req.FileKey)
		if err != nil {
			handleError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"url":        url,
			"file_key":   req.FileKey,
			"method":     "GET",
			"expires_in": 604800,
		})
	}
}

// checkConvPermission 检查用户是否有权限访问指定会话（上传/下载通用）
func (h *Handler) checkConvPermission(ctx context.Context, convID string, userID int64) error {
	hasPerm, err := h.svc.convPartStore.Exists(ctx, h.svc.convPartStore.DB(), convID, userID)
	if err != nil {
		return err
	}
	if !hasPerm {
		if groupID := ExtractGroupID(convID); groupID != "" {
			isMember, _ := h.svc.IsUserInGroup(ctx, groupID, userID)
			if !isMember {
				return ErrForbidden
			}
		} else {
			return ErrForbidden
		}
	}
	return nil
}
