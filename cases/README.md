# Integration Cases

This directory stores reusable incident fixtures for the OPS agent.

Each case should be readable by both humans and automation. The preferred
format is YAML with these sections:

- `schema_version`: fixture format version
- `id`: stable case identifier
- `title`: short case title
- `agent_profile`: expected agent kernel, model, and behavior
- `toolchain`: runtime tools the OPS agent is allowed to use
- `scenario`: topology, symptoms, and investigation objective
- `evidence`: representative commands, logs, and observations
- `tasks`: required investigation steps
- `expected_findings`: facts the agent should conclude
- `expected_actions`: remediation steps the agent should recommend or execute
- `closed_loop`: execution, verification, and feedback requirements
- `acceptance_criteria`: pass/fail conditions for the case

The goal is not to replay every shell command exactly. The goal is to give the
OPS agent enough context to:

1. Identify the real fault domain.
2. Correlate logs across hosts.
3. Separate symptoms from root cause.
4. Execute safe runtime remediation through approved tools such as
   `ssh-mcp-server`.
5. Verify the result and return a closed-loop status summary.
