package models

import (
	"time"

	"github.com/google/uuid"
)

// JobHire 聘請記錄
type JobHire struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	VacancyID         *uuid.UUID `gorm:"type:uuid" json:"vacancy_id,omitempty"`
	ApplicantID       *uuid.UUID `gorm:"type:uuid" json:"applicant_id,omitempty"`
	CandidateName     string     `gorm:"type:varchar(255);not null" json:"candidate_name"`
	CandidateLastName string     `gorm:"type:varchar(255)" json:"candidate_last_name,omitempty"`
	Email             string     `gorm:"type:varchar(255)" json:"email,omitempty"`
	Phone             string     `gorm:"type:varchar(50)" json:"phone,omitempty"`
	Status            string     `gorm:"type:varchar(20);not null;default:'applied'" json:"status"` // applied/interview/offered/hired/rejected
	StartDate         *time.Time `json:"start_date,omitempty"`
	Notes             string     `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`

	Vacancy *JobVacancy `gorm:"foreignKey:VacancyID;references:ID" json:"vacancy,omitempty"`
	Applicant *JobApplicant `gorm:"foreignKey:ApplicantID;references:ID" json:"applicant,omitempty"`
}

func (JobHire) TableName() string {
	return "job_hires"
}


