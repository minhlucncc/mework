package mcp

import "context"

// CreateComment posts a comment to a ticket via the MCP create_comment tool.
func (m *Client) CreateComment(ctx context.Context, ticketID, body string) (string, error) {
	return m.call(ctx, "create_comment", map[string]any{
		"ticket_id": ticketID,
		"body":      body,
	})
}

// CreateChecklist creates a checklist on a ticket.
func (m *Client) CreateChecklist(ctx context.Context, ticketID, title string) (string, error) {
	return m.call(ctx, "create_checklist", map[string]any{
		"ticket_id": ticketID,
		"title":     title,
	})
}

// CreateChecklistItem adds an item to a checklist.
func (m *Client) CreateChecklistItem(ctx context.Context, checklistID, title string) (string, error) {
	return m.call(ctx, "create_checklist_item", map[string]any{
		"checklist_id": checklistID,
		"title":        title,
	})
}

// UpdateChecklistItem toggles a checklist item's checked state.
func (m *Client) UpdateChecklistItem(ctx context.Context, itemID string, isChecked bool) (string, error) {
	return m.call(ctx, "update_checklist_item", map[string]any{
		"checklist_item_id": itemID,
		"is_checked":        isChecked,
	})
}
