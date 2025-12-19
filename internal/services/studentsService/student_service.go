package studentsService

import (
	"context"

	"github.com/Domenick1991/students/internal/models"
)

type StudentStorage interface {
	GetStudentInfoByIDs(ctx context.Context, IDs []uint64) ([]*models.StudentInfo, error)
	UpsertStudentInfo(ctx context.Context, studentInfos []*models.StudentInfo) error
}

type StudentService struct {
	studentStorage StudentStorage
	minNameLen     int
	maxNameLen     int
}

func NewStudentService(ctx context.Context, studentStorage StudentStorage, minNameLen, maxNameLen int) *StudentService {
	return &StudentService{
		studentStorage: studentStorage,
		minNameLen:     minNameLen,
		maxNameLen:     maxNameLen,
	}
}
