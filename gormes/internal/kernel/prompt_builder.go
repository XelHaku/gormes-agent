package kernel

import "github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"

func (k *Kernel) buildChatRequest(systemMsgs []hermes.Message) hermes.ChatRequest {
	msgs := append([]hermes.Message(nil), k.history...)
	if k.cfg.ContextEngine != nil {
		msgs = k.cfg.ContextEngine.PlanMessages(systemMsgs, msgs).Messages
	} else if len(systemMsgs) > 0 {
		assembled := make([]hermes.Message, 0, len(systemMsgs)+len(msgs))
		assembled = append(assembled, systemMsgs...)
		assembled = append(assembled, msgs...)
		msgs = assembled
	}

	request := hermes.ChatRequest{
		Model:     k.cfg.Model,
		SessionID: k.sessionID,
		Stream:    true,
		Messages:  msgs,
	}
	if k.cfg.Tools != nil {
		descs := k.cfg.Tools.Descriptors()
		wireDescs := make([]hermes.ToolDescriptor, len(descs))
		for i, d := range descs {
			wireDescs[i] = hermes.ToolDescriptor{Name: d.Name, Description: d.Description, Schema: d.Schema}
		}
		request.Tools = wireDescs
	}
	return request
}
