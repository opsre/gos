package bootstrap

import (
	"context"
	"fmt"
	"log"
	"time"

	argocdclient "gos/internal/infrastructure/argocd"
)

// CheckArgoCDConnection 检查业务状态并返回校验结果。
func CheckArgoCDConnection(cfg Config) error {
	if !cfg.ArgoCD.Enabled {
		return nil
	}
	if !cfg.ArgoCD.StartupCheckEnabled {
		log.Println("argocd is enabled, startup check is disabled")
		return nil
	}
	if cfg.ArgoCD.BaseURL == "" {
		log.Println("argocd is enabled, startup check skipped because base_url is empty; expecting DB-managed instances")
		return nil
	}
	log.Println("argocd:", cfg.ArgoCD.BaseURL)

	client := argocdclient.NewClient(argocdclient.Config{
		BaseURL:            cfg.ArgoCD.BaseURL,
		InsecureSkipVerify: cfg.ArgoCD.InsecureSkipVerify,
		AuthMode:           cfg.ArgoCD.AuthMode,
		Token:              cfg.ArgoCD.Token,
		Username:           cfg.ArgoCD.Username,
		Password:           cfg.ArgoCD.Password,
		TimeoutSec:         cfg.ArgoCD.TimeoutSec,
	})

	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ArgoCD.TimeoutSec)*time.Second)
		lastErr = client.Ping(ctx)
		cancel()
		if lastErr == nil {
			return nil
		}
		if attempt < 5 {
			time.Sleep(2 * time.Second)
		}
	}

	return fmt.Errorf("argocd startup connection check failed after %d attempts (base_url=%s): %w", 5, cfg.ArgoCD.BaseURL, lastErr)
}
