// REST API types
export interface Artifact {
  id: string
  run_id: string
  agent_task_id: string | null
  name: string
  mime_type: string
  size_bytes: number
  storage_path: string
  created_at: string
}

export interface APIKey {
  id: string
  name: string
  key_prefix: string
  created_at: string
  last_used_at?: string
  expires_at?: string
}

export interface AdminUser {
  id: string
  email: string
  display_name: string
  is_admin: boolean
  created_at: string
}

export interface AdminRun extends TeamRun {
  team_id: string
  initiated_by: string
}

export interface User {
  id: string
  email: string
  display_name: string
  is_admin: boolean
}

export interface Team {
  id: string
  name: string
  description: string
  created_at: string
}

export interface TeamRun {
  id: string
  team_id: string
  objective: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
  created_at: string
}

export interface AgentTask {
  id: string
  run_id: string
  agent_role: string
  status: string
}

// WebSocket event types — discriminated union for live run stream
export type RunEvent =
  | {
      type: 'agent_thinking'
      agent_task_id: string
      agent_role: string
      timestamp: string
      payload: { thought: string }
    }
  | {
      type: 'agent_action'
      agent_task_id: string
      agent_role: string
      timestamp: string
      payload: { tool: string; input: unknown; output?: unknown }
    }
  | {
      type: 'agent_message'
      agent_task_id: string
      agent_role: string
      timestamp: string
      payload: { content: string; to?: string }
    }
  | {
      type: 'agent_done'
      agent_task_id: string
      agent_role: string
      timestamp: string
      payload: { summary: string; artifact_ids?: string[] }
    }
  | {
      type: 'agent_error'
      agent_task_id: string
      agent_role: string
      timestamp: string
      payload: { error: string }
    }
  | {
      type: 'run_complete'
      run_id: string
      timestamp: string
      payload: { summary: string; artifact_ids: string[]; duration_ms: number }
    }
  | {
      type: 'run_error'
      run_id: string
      timestamp: string
      payload: { error: string }
    }
  | { type: 'buffer_overflow'; timestamp: string }
