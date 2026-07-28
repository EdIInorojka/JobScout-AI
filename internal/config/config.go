package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"jobscout.ai/internal/core"
)

type Config struct {
	Environment       string
	HTTPAddr          string
	DatabaseURL       string
	AppName           string
	OwnerEmail        string
	HHUserAgent       string
	HHAccessToken     string
	HHBaseURL         string
	HHPageSize        int
	HHMaxPages        int
	HHMinDelayMS      int
	MaxVacancyAgeDays int
	SearchTimeoutSec  int
	EnableTelegram    bool
	TelegramBotToken  string
	TelegramOwnerID   int64
	Scoring           core.ScoringConfig
}

func Load() (Config, error) {
	cfg := Config{
		Environment:       getEnv("APP_ENV", "development"),
		HTTPAddr:          getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:       strings.TrimSpace(os.Getenv("DATABASE_URL")),
		AppName:           getEnv("APP_NAME", "JobScout AI"),
		OwnerEmail:        getEnv("OWNER_EMAIL", "owner@example.com"),
		HHUserAgent:       strings.TrimSpace(os.Getenv("HH_USER_AGENT")),
		HHAccessToken:     strings.TrimSpace(os.Getenv("HH_ACCESS_TOKEN")),
		HHBaseURL:         getEnv("HH_BASE_URL", "https://api.hh.ru"),
		HHPageSize:        getEnvInt("HH_PAGE_SIZE", 20),
		HHMaxPages:        getEnvInt("HH_MAX_PAGES", 3),
		HHMinDelayMS:      getEnvInt("HH_MIN_DELAY_MS", 500),
		MaxVacancyAgeDays: getEnvInt("MAX_VACANCY_AGE_DAYS", 45),
		SearchTimeoutSec:  getEnvInt("SEARCH_TIMEOUT_SEC", 45),
		EnableTelegram:    getEnvBool("ENABLE_TELEGRAM", true),
		TelegramBotToken:  strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramOwnerID:   getEnvInt64("TELEGRAM_OWNER_ID", 0),
		Scoring:           core.DefaultScoringConfig(),
	}

	if raw := strings.TrimSpace(os.Getenv("SCORING_VERSION")); raw != "" {
		cfg.Scoring.Version = raw
	}
	cfg.Scoring.Weights.RoleWeight = getEnvInt("SCORING_ROLE_WEIGHT", cfg.Scoring.Weights.RoleWeight)
	cfg.Scoring.Weights.SkillsWeight = getEnvInt("SCORING_SKILLS_WEIGHT", cfg.Scoring.Weights.SkillsWeight)
	cfg.Scoring.Weights.ExperienceWeight = getEnvInt("SCORING_EXPERIENCE_WEIGHT", cfg.Scoring.Weights.ExperienceWeight)
	cfg.Scoring.Weights.GradeWeight = getEnvInt("SCORING_GRADE_WEIGHT", cfg.Scoring.Weights.GradeWeight)
	cfg.Scoring.Weights.LocationWeight = getEnvInt("SCORING_LOCATION_WEIGHT", cfg.Scoring.Weights.LocationWeight)
	cfg.Scoring.Weights.SalaryWeight = getEnvInt("SCORING_SALARY_WEIGHT", cfg.Scoring.Weights.SalaryWeight)
	cfg.Scoring.Weights.DomainWeight = getEnvInt("SCORING_DOMAIN_WEIGHT", cfg.Scoring.Weights.DomainWeight)
	cfg.Scoring.ApplyThreshold = getEnvInt("SCORING_APPLY_THRESHOLD", cfg.Scoring.ApplyThreshold)
	cfg.Scoring.ReviewThreshold = getEnvInt("SCORING_REVIEW_THRESHOLD", cfg.Scoring.ReviewThreshold)

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.HHUserAgent == "" {
		cfg.HHUserAgent = fmt.Sprintf("%s/1.0 (%s)", cfg.AppName, cfg.OwnerEmail)
	}
	if err := cfg.Scoring.Validate(); err != nil {
		return Config{}, err
	}
	if cfg.EnableTelegram && cfg.TelegramBotToken == "" {
		cfg.EnableTelegram = false
	}
	if cfg.EnableTelegram && cfg.TelegramOwnerID == 0 {
		return Config{}, errors.New("TELEGRAM_OWNER_ID is required when ENABLE_TELEGRAM is true")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}
