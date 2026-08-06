package scheduler

import (
	"time"

	"github.com/go-co-op/gocron"
)

var scheduler *Scheduler

func GetScheduler() *Scheduler {
	return scheduler
}

type Scheduler struct {
	scheduler *gocron.Scheduler
}

func InitScheduler() *Scheduler {
	newScheduler := gocron.NewScheduler(time.Local)
	scheduler = &Scheduler{
		scheduler: newScheduler,
	}

	return scheduler
}

func (s *Scheduler) Cron(cron string, jobFunc any) (*gocron.Job, error) {
	job, err := s.scheduler.Cron(cron).Do(jobFunc)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Scheduler) EveryMinute(jobFunc any) (*gocron.Job, error) {
	job, err := s.scheduler.Every(1).Minutes().Do(jobFunc)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Scheduler) EveryFiveMinutes(jobFunc any) (*gocron.Job, error) {
	job, err := s.scheduler.Every(5).Minutes().Do(jobFunc)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Scheduler) EveryTenMinutes(jobFunc any) (*gocron.Job, error) {
	job, err := s.scheduler.Every(10).Minutes().Do(jobFunc)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Scheduler) EveryHour(jobFunc any) (*gocron.Job, error) {
	job, err := s.scheduler.Every(1).Hours().Do(jobFunc)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Scheduler) EveryDay(jobFunc any) (*gocron.Job, error) {
	job, err := s.scheduler.Every(1).Days().Do(jobFunc)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Scheduler) EveryDayAt(at any, jobFunc any) (*gocron.Job, error) {
	job, err := s.scheduler.Every(1).Days().At(at).Do(jobFunc)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Scheduler) Start() {
	s.scheduler.StartAsync()
}
