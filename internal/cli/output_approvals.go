package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type mergeRequestApprovalUserOutput struct {
	Username   string `json:"username" toon:"username"`
	ApprovedAt string `json:"approved_at,omitempty" toon:"approved_at,omitempty"`
}

type mergeRequestApprovalCompactOutput struct {
	IID               int64                            `json:"iid,omitempty" toon:"iid,omitempty"`
	Approved          bool                             `json:"approved" toon:"approved"`
	ApprovalsRequired int64                            `json:"approvals_required" toon:"approvals_required"`
	ApprovalsLeft     int64                            `json:"approvals_left" toon:"approvals_left"`
	UserHasApproved   bool                             `json:"user_has_approved" toon:"user_has_approved"`
	UserCanApprove    bool                             `json:"user_can_approve" toon:"user_can_approve"`
	ApprovedBy        []mergeRequestApprovalUserOutput `json:"approved_by" toon:"approved_by"`
}

type mergeRequestApprovalRuleOutput struct {
	ID                int64    `json:"id" toon:"id"`
	Name              string   `json:"name" toon:"name"`
	RuleType          string   `json:"rule_type" toon:"rule_type"`
	ApprovalsRequired int64    `json:"approvals_required" toon:"approvals_required"`
	Approved          bool     `json:"approved" toon:"approved"`
	ApprovedBy        []string `json:"approved_by" toon:"approved_by"`
}

type mergeRequestApprovalFullOutput struct {
	IID                            int64                            `json:"iid" toon:"iid"`
	Title                          string                           `json:"title" toon:"title"`
	State                          string                           `json:"state" toon:"state"`
	MergeStatus                    string                           `json:"merge_status" toon:"merge_status"`
	Approved                       bool                             `json:"approved" toon:"approved"`
	ApprovalsBeforeMerge           int64                            `json:"approvals_before_merge" toon:"approvals_before_merge"`
	ApprovalsRequired              int64                            `json:"approvals_required" toon:"approvals_required"`
	ApprovalsLeft                  int64                            `json:"approvals_left" toon:"approvals_left"`
	RequirePasswordToApprove       bool                             `json:"require_password_to_approve" toon:"require_password_to_approve"`
	UserHasApproved                bool                             `json:"user_has_approved" toon:"user_has_approved"`
	UserCanApprove                 bool                             `json:"user_can_approve" toon:"user_can_approve"`
	ApprovedBy                     []mergeRequestApprovalUserOutput `json:"approved_by" toon:"approved_by"`
	SuggestedApprovers             []string                         `json:"suggested_approvers" toon:"suggested_approvers"`
	Approvers                      []mergeRequestApprovalUserOutput `json:"approvers" toon:"approvers"`
	ApproverGroups                 []string                         `json:"approver_groups" toon:"approver_groups"`
	ApprovalRulesLeft              []mergeRequestApprovalRuleOutput `json:"approval_rules_left" toon:"approval_rules_left"`
	HasApprovalRules               bool                             `json:"has_approval_rules" toon:"has_approval_rules"`
	MergeRequestApproversAvailable bool                             `json:"merge_request_approvers_available" toon:"merge_request_approvers_available"`
	MultipleApprovalRulesAvailable bool                             `json:"multiple_approval_rules_available" toon:"multiple_approval_rules_available"`
}

type axiMergeRequestApprovalOutput struct {
	Approval any      `json:"approval" toon:"approval"`
	Help     []string `json:"help,omitempty" toon:"help,omitempty"`
}

func writeMergeRequestApproval(w io.Writer, format string, mode commandMode, approvals *gitlab.MergeRequestApprovals, full bool, help []string) error {
	if approvals == nil {
		return errors.New("gitlab api returned an empty merge request approvals response")
	}

	if mode == commandModeAxi {
		var out any = mergeRequestApprovalCompactFromAPI(approvals)
		if full {
			out = mergeRequestApprovalFullFromAPI(approvals)
		}

		return writeAxi(w, format, axiMergeRequestApprovalOutput{Approval: out, Help: help})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if full {
		out := mergeRequestApprovalFullFromAPI(approvals)
		if format == "json" {
			return writeJSON(w, out)
		}

		return writeMergeRequestApprovalFullText(w, out)
	}

	out := mergeRequestApprovalCompactFromAPI(approvals)
	if format == "json" {
		return writeJSON(w, out)
	}

	return writeMergeRequestApprovalCompactText(w, out)
}

func writeMergeRequestApprovalCompactText(w io.Writer, out mergeRequestApprovalCompactOutput) error {
	if out.IID > 0 {
		if _, err := fmt.Fprintf(w, "iid: %d\n", out.IID); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintf(
		w,
		"approved: %t\napprovals_required: %d\napprovals_left: %d\nuser_has_approved: %t\nuser_can_approve: %t\napproved_by: %s\n",
		out.Approved,
		out.ApprovalsRequired,
		out.ApprovalsLeft,
		out.UserHasApproved,
		out.UserCanApprove,
		strings.Join(approvalUserText(out.ApprovedBy), ", "),
	)

	return err
}

func writeMergeRequestApprovalFullText(w io.Writer, out mergeRequestApprovalFullOutput) error {
	if _, err := fmt.Fprintf(
		w,
		"iid: %d\ntitle: %s\nstate: %s\nmerge_status: %s\napproved: %t\napprovals_before_merge: %d\napprovals_required: %d\napprovals_left: %d\nrequire_password_to_approve: %t\nuser_has_approved: %t\nuser_can_approve: %t\napproved_by: %s\nsuggested_approvers: %s\napprovers: %s\napprover_groups: %s\nhas_approval_rules: %t\nmerge_request_approvers_available: %t\nmultiple_approval_rules_available: %t\n",
		out.IID,
		out.Title,
		out.State,
		out.MergeStatus,
		out.Approved,
		out.ApprovalsBeforeMerge,
		out.ApprovalsRequired,
		out.ApprovalsLeft,
		out.RequirePasswordToApprove,
		out.UserHasApproved,
		out.UserCanApprove,
		strings.Join(approvalUserText(out.ApprovedBy), ", "),
		strings.Join(out.SuggestedApprovers, ", "),
		strings.Join(approvalUserText(out.Approvers), ", "),
		strings.Join(out.ApproverGroups, ", "),
		out.HasApprovalRules,
		out.MergeRequestApproversAvailable,
		out.MultipleApprovalRulesAvailable,
	); err != nil {
		return err
	}

	if len(out.ApprovalRulesLeft) == 0 {
		_, err := fmt.Fprintln(w, "approval_rules_left: none")
		return err
	}

	if _, err := fmt.Fprintln(w, "approval_rules_left:"); err != nil {
		return err
	}
	for _, rule := range out.ApprovalRulesLeft {
		if _, err := fmt.Fprintf(
			w,
			"- id=%d name=%q rule_type=%s approvals_required=%d approved=%t approved_by=%s\n",
			rule.ID,
			rule.Name,
			rule.RuleType,
			rule.ApprovalsRequired,
			rule.Approved,
			strings.Join(rule.ApprovedBy, ","),
		); err != nil {
			return err
		}
	}

	return nil
}

func mergeRequestApprovalCompactFromAPI(approvals *gitlab.MergeRequestApprovals) mergeRequestApprovalCompactOutput {
	out := mergeRequestApprovalCompactOutput{
		IID:               approvals.IID,
		Approved:          approvals.Approved,
		ApprovalsRequired: approvals.ApprovalsRequired,
		ApprovalsLeft:     approvals.ApprovalsLeft,
		UserHasApproved:   approvals.UserHasApproved,
		UserCanApprove:    approvals.UserCanApprove,
		ApprovedBy:        approvedByUsers(approvals.ApprovedBy),
	}
	if out.ApprovedBy == nil {
		out.ApprovedBy = []mergeRequestApprovalUserOutput{}
	}

	return out
}

func mergeRequestApprovalFullFromAPI(approvals *gitlab.MergeRequestApprovals) mergeRequestApprovalFullOutput {
	out := mergeRequestApprovalFullOutput{
		IID:                            approvals.IID,
		Title:                          approvals.Title,
		State:                          approvals.State,
		MergeStatus:                    approvals.MergeStatus,
		Approved:                       approvals.Approved,
		ApprovalsBeforeMerge:           approvals.ApprovalsBeforeMerge,
		ApprovalsRequired:              approvals.ApprovalsRequired,
		ApprovalsLeft:                  approvals.ApprovalsLeft,
		RequirePasswordToApprove:       approvals.RequirePasswordToApprove,
		UserHasApproved:                approvals.UserHasApproved,
		UserCanApprove:                 approvals.UserCanApprove,
		ApprovedBy:                     approvedByUsers(approvals.ApprovedBy),
		SuggestedApprovers:             basicUsernames(approvals.SuggestedApprovers),
		Approvers:                      approvedByUsers(approvals.Approvers),
		ApproverGroups:                 approverGroupNames(approvals.ApproverGroups),
		ApprovalRulesLeft:              approvalRuleOutputs(approvals.ApprovalRulesLeft),
		HasApprovalRules:               approvals.HasApprovalRules,
		MergeRequestApproversAvailable: approvals.MergeRequestApproversAvailable,
		MultipleApprovalRulesAvailable: approvals.MultipleApprovalRulesAvailable,
	}
	if out.ApprovedBy == nil {
		out.ApprovedBy = []mergeRequestApprovalUserOutput{}
	}
	if out.SuggestedApprovers == nil {
		out.SuggestedApprovers = []string{}
	}
	if out.Approvers == nil {
		out.Approvers = []mergeRequestApprovalUserOutput{}
	}
	if out.ApproverGroups == nil {
		out.ApproverGroups = []string{}
	}
	if out.ApprovalRulesLeft == nil {
		out.ApprovalRulesLeft = []mergeRequestApprovalRuleOutput{}
	}

	return out
}

func approvedByUsers(users []*gitlab.MergeRequestApproverUser) []mergeRequestApprovalUserOutput {
	out := make([]mergeRequestApprovalUserOutput, 0, len(users))
	for _, user := range users {
		if user == nil || user.User == nil {
			continue
		}
		out = append(out, mergeRequestApprovalUserOutput{
			Username:   basicUsername(user.User),
			ApprovedAt: formatTimeValue(user.ApprovedAt),
		})
	}

	return out
}

func approverGroupNames(groups []*gitlab.MergeRequestApproverGroup) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}

		nested := group.Group
		name := nested.FullPath
		if name == "" {
			name = nested.FullName
		}
		if name == "" {
			name = nested.Path
		}
		if name == "" {
			name = nested.Name
		}
		if name != "" {
			names = append(names, name)
		}
	}

	return names
}

func approvalRuleOutputs(rules []*gitlab.MergeRequestApprovalRule) []mergeRequestApprovalRuleOutput {
	out := make([]mergeRequestApprovalRuleOutput, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		approvedBy := basicUsernames(rule.ApprovedBy)
		if approvedBy == nil {
			approvedBy = []string{}
		}
		out = append(out, mergeRequestApprovalRuleOutput{
			ID:                rule.ID,
			Name:              rule.Name,
			RuleType:          rule.RuleType,
			ApprovalsRequired: rule.ApprovalsRequired,
			Approved:          rule.Approved,
			ApprovedBy:        approvedBy,
		})
	}

	return out
}

func approvalUserText(users []mergeRequestApprovalUserOutput) []string {
	out := make([]string, 0, len(users))
	for _, user := range users {
		if user.Username == "" {
			continue
		}
		if user.ApprovedAt != "" {
			out = append(out, fmt.Sprintf("%s (%s)", user.Username, user.ApprovedAt))
			continue
		}
		out = append(out, user.Username)
	}

	return out
}
