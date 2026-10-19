package binding

import (
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// ReviewUnknownAgentTask only changes owner-reviewed disposition evidence.
// It never re-executes an Agent or changes the original unknown outcome.
func (s *Service) ReviewUnknownAgentTask(id, handling string) (AgentTask, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentTask{}, err
	}
	taskID, err := agentdomain.ParseTaskID(id)
	if err != nil {
		return AgentTask{}, err
	}
	task, err := s.runtime.Agents.ReviewUnknownTask(s.ctx, taskID, handling, caller)
	if err != nil {
		return AgentTask{}, err
	}
	return projectTask(task), nil
}

// ResumeAgentTask is an explicit new owner-origin turn. Historical unknown
// and Profile-origin Tasks cannot be executed through this owner facade.
func (s *Service) ResumeAgentTask(id, prompt string) (AgentTask, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentTask{}, err
	}
	taskID, err := agentdomain.ParseTaskID(id)
	if err != nil {
		return AgentTask{}, err
	}
	source, err := s.runtime.Agents.Task(s.ctx, taskID, caller)
	if err != nil {
		return AgentTask{}, err
	}
	task, err := s.runtime.Agents.Resume(s.ctx, taskID, prompt, source.AgentID, caller)
	if err != nil {
		return AgentTask{}, err
	}
	return projectTask(task), nil
}
