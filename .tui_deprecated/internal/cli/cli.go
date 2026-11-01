package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"controls/execution"
	"controls/internal/control"
	"controls/internal/dashboard"
	"controls/internal/sessions"
	"controls/internal/telemetry"
)

type option struct {
	name  string
	value bool
}

var outputOption = map[string]option{"-o": {"output", true}, "--output": {"output", true}}

func Help(w io.Writer) {
	fmt.Fprint(w, `Factory executes configured agent workflows.

  factory exec [brief] [-f file ...] [-c factory.json] [--context ID]
  factory ls [--context ID] [-o json]
  factory get-context | set-context ID | contexts
  factory inbox [--context ID]
  factory monitor
  factory report [EXECUTION-ID] [--factory NAME] [--from DATE] [--to DATE] [-o html|json|template]
  factory report --snapshot ID [-o html|json]
  factory mcp
  factory ID [-o json | stream [-f] | attach | logs [-f] | events [-f] | messages | send TEXT | send --file FILE]
  factory retry STEP-OR-HOOK-ID [--reason TEXT]
  factory answer REQUEST-ID OUTCOME [--reason TEXT]
  factory cancel EXECUTION-ID

An ID names one execution, step visit, hook invocation, session, or decision request.
The selected context starts as * (all contexts); explicit IDs work in any context.
`)
}

func Run(ctx context.Context, args []string, client *control.Client, sessionClient *sessions.Client, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		Help(stdout)
		return nil
	}
	switch args[0] {
	case "report":
		return reportCommand(ctx, args[1:], client, stdout)
	case "mcp":
		if len(args) != 1 {
			return errors.New("usage: factory mcp")
		}
		return serveOperatorMCP(ctx, client)
	case "exec":
		return execCommand(ctx, args[1:], client, stdin, stdout)
	case "ls":
		return listCommand(ctx, args[1:], client, stdout)
	case "get-context":
		return getContextCommand(ctx, args[1:], client, stdout)
	case "set-context":
		if len(args) != 2 {
			return errors.New("usage: factory set-context ID (use * for all)")
		}
		selected, err := client.SetContext(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, selected.ID)
		return nil
	case "contexts":
		items, err := client.Contexts(ctx)
		if err != nil {
			return err
		}
		writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, "CONTEXT ID\tDESCRIPTION")
		for _, item := range items {
			fmt.Fprintf(writer, "%s\t%s\n", item.ID, item.Description)
		}
		return writer.Flush()
	case "inbox":
		return inboxCommand(ctx, args[1:], client, stdout)
	case "monitor":
		if len(args) != 1 {
			return errors.New("usage: factory monitor")
		}
		return dashboard.Run(ctx, client, sessionClient)
	case "retry":
		return retryCommand(ctx, args[1:], client, stdout)
	case "answer":
		return answerCommand(ctx, args[1:], client, stdout)
	case "cancel":
		if len(args) != 2 {
			return errors.New("usage: factory cancel EXECUTION-ID")
		}
		record, err := client.Cancel(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s  %s\n", record.ID, record.State)
		return nil
	default:
		return resourceCommand(ctx, args, client, sessionClient, stdin, stdout)
	}
}

func execCommand(ctx context.Context, args []string, client *control.Client, stdin io.Reader, stdout io.Writer) error {
	positionals, options, err := parse(args, map[string]option{
		"-c": {"config", true}, "--config": {"config", true},
		"-f": {"file", true}, "--file": {"file", true},
		"--context": {"context", true},
	})
	if err != nil {
		return err
	}
	if len(positionals) > 1 {
		return errors.New("exec accepts one brief; use repeated --file for documents")
	}
	brief := ""
	if len(positionals) == 1 {
		brief = positionals[0]
	}
	if strings.TrimSpace(brief) == "" && len(options["file"]) == 0 {
		return errors.New("exec needs a brief or --file")
	}
	configPath := last(options["config"])
	if configPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		configPath = filepath.Join(cwd, "factory.json")
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("factory.json not found at %s; pass -c /path/to/factory.json: %w", configPath, err)
	}
	files := make([]string, 0, len(options["file"]))
	var stdinData []byte
	for _, path := range options["file"] {
		if path == "-" {
			if stdinData != nil {
				return errors.New("stdin may be supplied only once")
			}
			stdinData, err = io.ReadAll(io.LimitReader(stdin, (32<<20)+1))
			if err != nil {
				return err
			}
			if len(stdinData) > (32 << 20) {
				return errors.New("stdin exceeds 32 MiB")
			}
			files = append(files, "-")
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		files = append(files, absolute)
	}
	record, err := client.Exec(ctx, control.ExecRequest{
		ConfigPath: configPath, ContextID: last(options["context"]), Brief: brief,
		Files: files, Stdin: stdinData, Actor: actor(),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "CONTEXT %s\nEXECUTION %s\nCONFIG %s\nSTATE %s\n", record.ContextID, record.ID, record.ConfigPath, record.State)
	if record.State == execution.ExecutionFailed {
		return errors.New(record.Error)
	}
	return nil
}

func listCommand(ctx context.Context, args []string, client *control.Client, stdout io.Writer) error {
	positionals, options, err := parse(args, merge(outputOption, map[string]option{"--context": {"context", true}}))
	if err != nil {
		return err
	}
	if len(positionals) != 0 {
		return errors.New("usage: factory ls [--context ID]")
	}
	items, err := client.List(ctx, last(options["context"]))
	if err != nil {
		return err
	}
	if last(options["output"]) == "json" {
		return printJSON(stdout, items)
	}
	contexts, err := client.Contexts(ctx)
	if err != nil {
		return err
	}
	descriptions := make(map[string]string, len(contexts))
	for _, item := range contexts {
		descriptions[item.ID] = item.Description
	}
	writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "EXECUTION ID\tCONTEXT ID\tCONTEXT\tSTATE\tNOW\tLAST ACTIVITY\tACTION")
	for _, item := range items {
		current := "—"
		for _, step := range item.Steps {
			if step.ID == item.CurrentStepID {
				current = string(step.Key)
				break
			}
		}
		for _, hook := range item.Hooks {
			if hook.ID == item.CurrentStepID {
				current = "hook/" + hook.Context.Name
			}
		}
		action := "—"
		for _, request := range item.Requests {
			if request.State == "pending" {
				action = "answer " + request.ID
				break
			}
		}
		if action == "—" && item.State == execution.ExecutionBlocked {
			action = "retry " + item.CurrentStepID
		}
		contextName := descriptions[item.ContextID]
		if contextName == "" {
			contextName = item.Brief
		}
		if contextName == "" && len(item.Input.Documents) > 0 {
			contextName = item.Input.Documents[0].Name
		}
		contextName = firstLine(contextName, 40)
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", item.ID, item.ContextID, contextName, item.State, current, item.UpdatedAt.Local().Format("Jan 02 15:04"), action)
	}
	return writer.Flush()
}

func getContextCommand(ctx context.Context, args []string, client *control.Client, stdout io.Writer) error {
	positionals, options, err := parse(args, outputOption)
	if err != nil {
		return err
	}
	if len(positionals) != 0 {
		return errors.New("usage: factory get-context")
	}
	selected, err := client.GetContext(ctx)
	if err != nil {
		return err
	}
	if last(options["output"]) == "json" {
		return printJSON(stdout, selected)
	}
	if selected.Context == nil {
		fmt.Fprintln(stdout, "* (all contexts)")
	} else {
		fmt.Fprintf(stdout, "%s  %s\n", selected.ID, selected.Context.Description)
	}
	return nil
}

func inboxCommand(ctx context.Context, args []string, client *control.Client, stdout io.Writer) error {
	positionals, options, err := parse(args, merge(outputOption, map[string]option{"--context": {"context", true}}))
	if err != nil {
		return err
	}
	if len(positionals) != 0 {
		return errors.New("usage: factory inbox [--context ID]")
	}
	items, err := client.Inbox(ctx, last(options["context"]))
	if err != nil {
		return err
	}
	if last(options["output"]) == "json" {
		return printJSON(stdout, items)
	}
	writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "REQUEST ID\tEXECUTION ID\tPROMPT\tOPTIONS")
	for _, item := range items {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", item.ID, item.ExecutionID, firstLine(item.Prompt, 48), item.Options)
	}
	return writer.Flush()
}

func retryCommand(ctx context.Context, args []string, client *control.Client, stdout io.Writer) error {
	positionals, options, err := parse(args, map[string]option{"--reason": {"reason", true}})
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("usage: factory retry STEP-OR-HOOK-ID [--reason TEXT]")
	}
	record, err := client.Retry(ctx, positionals[0], last(options["reason"]), actor())
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s  %s\n", record.ID, record.State)
	return nil
}

func answerCommand(ctx context.Context, args []string, client *control.Client, stdout io.Writer) error {
	positionals, options, err := parse(args, map[string]option{"--reason": {"reason", true}})
	if err != nil {
		return err
	}
	if len(positionals) != 2 {
		return errors.New("usage: factory answer REQUEST-ID OUTCOME [--reason TEXT]")
	}
	record, err := client.Answer(ctx, positionals[0], execution.Outcome(positionals[1]), last(options["reason"]), actor())
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s  %s\n", record.ID, record.State)
	return nil
}

func resourceCommand(ctx context.Context, args []string, client *control.Client, sessionClient *sessions.Client, stdin io.Reader, stdout io.Writer) error {
	id := args[0]
	if len(args) == 1 || strings.HasPrefix(args[1], "-") {
		positionals, options, err := parse(args[1:], outputOption)
		if err != nil {
			return err
		}
		if len(positionals) != 0 {
			return errors.New("usage: factory ID [-o json]")
		}
		return showResource(ctx, id, client, stdout, last(options["output"]) == "json")
	}
	switch args[1] {
	case "stream":
		positionals, options, err := parse(args[2:], map[string]option{"-f": {"follow", false}, "--follow": {"follow", false}})
		if err != nil {
			return err
		}
		if len(positionals) != 0 {
			return errors.New("usage: factory ID stream [-f]")
		}
		return streamActivity(ctx, id, client, stdout, len(options["follow"]) > 0, "")
	case "attach":
		if len(args) != 2 {
			return errors.New("usage: factory EXECUTION-ID|STEP-ID|SESSION-ID attach")
		}
		resource, err := client.Resource(ctx, id)
		if err != nil {
			return err
		}
		if resource.Type == "execution" {
			for _, step := range resource.Execution.Steps {
				if step.ID == resource.Execution.CurrentStepID && len(step.Sessions) > 0 {
					id = step.Sessions[len(step.Sessions)-1]
				}
			}
		}
		if resource.Type == "step" && len(resource.Step.Sessions) > 0 {
			id = resource.Step.Sessions[len(resource.Step.Sessions)-1]
		}
		if resource.Type != "session" {
			resource, err = client.Resource(ctx, id)
			if err != nil {
				return err
			}
		}
		if resource.Type != "session" {
			return errors.New("no agent session is available to attach")
		}
		if resource.Session.State != sessions.Running {
			return errors.New("agent session is not running")
		}
		fmt.Fprintf(stdout, "Attached to %s. Type a message and press Enter; Ctrl-C detaches.\n", id)
		go func() {
			scanner := bufio.NewScanner(stdin)
			scanner.Buffer(make([]byte, 4096), 16384)
			for scanner.Scan() {
				if _, sendErr := sessionClient.Send(ctx, id, scanner.Text()); sendErr != nil {
					fmt.Fprintf(stdout, "factory: send: %v\n", sendErr)
				}
			}
		}()
		return streamActivity(ctx, id, client, stdout, true, id)
	case "logs":
		positionals, options, err := parse(args[2:], map[string]option{"-f": {"follow", false}, "--follow": {"follow", false}})
		if err != nil {
			return err
		}
		if len(positionals) != 0 {
			return errors.New("usage: factory ID logs [-f]")
		}
		return streamLogs(ctx, id, client, stdout, len(options["follow"]) > 0)
	case "events":
		positionals, options, err := parse(args[2:], map[string]option{"-f": {"follow", false}, "--follow": {"follow", false}})
		if err != nil {
			return err
		}
		if len(positionals) != 0 {
			return errors.New("usage: factory ID events [-f]")
		}
		return streamEvents(ctx, id, client, stdout, len(options["follow"]) > 0)
	case "messages":
		positionals, options, err := parse(args[2:], outputOption)
		if err != nil {
			return err
		}
		if len(positionals) != 0 {
			return errors.New("usage: factory EXECUTION-OR-SESSION-ID messages")
		}
		resource, resourceErr := client.Resource(ctx, id)
		if resourceErr == nil && (resource.Type == "execution" || resource.Type == "step") {
			messages, err := client.Messages(ctx, resource.ExecutionID)
			if err != nil {
				return err
			}
			if last(options["output"]) == "json" {
				return printJSON(stdout, messages)
			}
			for _, message := range messages {
				body, _ := json.Marshal(message.Payload)
				fmt.Fprintf(stdout, "%s  %s  %s  %s\n", message.ID, message.At.Format(time.RFC3339), message.SenderName, body)
			}
			return nil
		}
		messages, err := sessionClient.Messages(ctx, id)
		if err != nil {
			return err
		}
		if last(options["output"]) == "json" {
			return printJSON(stdout, messages)
		}
		for _, message := range messages {
			fmt.Fprintf(stdout, "%s  %-9s  %s\n", message.ID, message.State, message.Body)
		}
		return nil
	case "send":
		positionals, options, err := parse(args[2:], map[string]option{"-f": {"file", true}, "--file": {"file", true}})
		if err != nil {
			return err
		}
		if len(positionals)+len(options["file"]) != 1 {
			return errors.New("usage: factory SESSION-ID send TEXT | send --file FILE")
		}
		body := ""
		if len(positionals) == 1 {
			body = positionals[0]
		} else {
			path := options["file"][0]
			var data []byte
			if path == "-" {
				data, err = io.ReadAll(io.LimitReader(stdin, 16385))
			} else {
				file, openErr := os.Open(path)
				if openErr != nil {
					return openErr
				}
				data, err = io.ReadAll(io.LimitReader(file, 16385))
				_ = file.Close()
			}
			if err != nil {
				return err
			}
			if len(data) > 16384 {
				return errors.New("message exceeds 16 KiB")
			}
			body = string(data)
		}
		message, err := sessionClient.Send(ctx, id, body)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s  %s\n", message.ID, message.State)
		return nil
	default:
		return fmt.Errorf("unknown resource operation %q", args[1])
	}
}

func showResource(ctx context.Context, id string, client *control.Client, stdout io.Writer, jsonOutput bool) error {
	resource, err := client.Resource(ctx, id)
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(stdout, resource)
	}
	switch resource.Type {
	case "execution":
		record := resource.Execution
		fmt.Fprintf(stdout, "Execution %s\nContext %s · %s\nWorkflow %s\nProject %s\nState %s\n", record.ID, record.ContextID, firstLine(record.Brief, 100), record.Workflow, record.Project, record.State)
		if record.Error != "" {
			fmt.Fprintf(stdout, "Error %s\n", publicText(record.Error))
		}
		printResources(stdout, record.Resources)
		if len(record.Hooks) > 0 {
			fmt.Fprintln(stdout, "\nHOOKS")
			for _, hook := range record.Hooks {
				fmt.Fprintf(stdout, "%s/%s %-20s %-10s try %d %s\n", hook.Context.Scope, hook.Context.Boundary, hook.Context.Name, hook.State, hook.Try, hook.ID)
				if hook.Error != "" {
					fmt.Fprintln(stdout, "  "+publicText(hook.Error))
				}
			}
		}
		fmt.Fprintln(stdout, "\nWORKFLOW")
		for _, definition := range record.Configuration.Workflow.Steps {
			stage := definition.Stage
			if stage == "" {
				stage = "—"
			}
			found := false
			for _, step := range record.Steps {
				if step.Key != definition.ID {
					continue
				}
				found = true
				fmt.Fprintf(stdout, "%-18s %-22s %-14s visit %d try %d  %s", stage, step.Key, step.State, step.Visit, step.Try, step.ID)
				if step.Error != "" {
					fmt.Fprintf(stdout, "  %s", publicText(step.Error))
				}
				fmt.Fprintln(stdout)
				printResources(stdout, step.Resources)
				for _, sessionID := range step.Sessions {
					fmt.Fprintf(stdout, "  agent session %s  (factory %s attach)\n", sessionID, sessionID)
				}
			}
			if !found {
				fmt.Fprintf(stdout, "%-18s %-22s pending\n", stage, definition.ID)
			}
		}
		for _, request := range record.Requests {
			if request.State == "pending" {
				fmt.Fprintf(stdout, "\nACTION answer %s (%s)\n", request.ID, request.Prompt)
			}
		}
		if record.State == execution.ExecutionBlocked {
			fmt.Fprintf(stdout, "\nACTION retry %s\n", record.CurrentStepID)
		}
	case "step":
		step := resource.Step
		fmt.Fprintf(stdout, "Step %s · %s\nExecution %s\nState %s · visit %d · try %d\n", step.ID, step.Key, resource.ExecutionID, step.State, step.Visit, step.Try)
		if step.Outcome != "" {
			fmt.Fprintf(stdout, "Outcome %s\n", step.Outcome)
		}
		if step.Error != "" {
			fmt.Fprintf(stdout, "Error %s\n", publicText(step.Error))
		}
		printResources(stdout, step.Resources)
		for _, session := range step.Sessions {
			fmt.Fprintf(stdout, "Session %s\n", session)
		}
	case "session":
		session := resource.Session
		fmt.Fprintf(stdout, "Session %s · %s\nExecution %s · step %s\nState %s · runner %s · PID %d\n", session.ID, session.Name, session.ExecutionID, session.StepID, session.State, session.Runner, session.PID)
		printResources(stdout, &session.Resources)
		end := time.Now()
		if session.EndedAt != nil {
			end = *session.EndedAt
		}
		fmt.Fprintf(stdout, "Elapsed %s\n", end.Sub(session.StartedAt).Truncate(time.Second))
		progress := session.StartedAt
		if session.Resources.LastProgressAt != nil {
			progress = *session.Resources.LastProgressAt
		}
		fmt.Fprintf(stdout, "Since progress %s\n", max(time.Duration(0), end.Sub(progress)).Truncate(time.Second))
		if command := session.Resources.Command(); command != "" && session.State == sessions.Running {
			fmt.Fprintf(stdout, "Current command %s\n", publicText(command))
		}
		for _, notice := range session.BudgetNotices {
			fmt.Fprintln(stdout, notice.Message)
		}
	case "hook":
		hook := resource.Hook
		fmt.Fprintf(stdout, "Hook %s · %s\nExecution %s\nScope %s · %s · stage %s · step %s\nState %s · try %d\n", hook.ID, hook.Context.Name, resource.ExecutionID, hook.Context.Scope, hook.Context.Boundary, hook.Context.Stage, hook.Context.Step, hook.State, hook.Try)
		if hook.SessionID != "" {
			fmt.Fprintf(stdout, "Session %s\n", hook.SessionID)
		}
		if hook.Error != "" {
			fmt.Fprintf(stdout, "Error %s\n", publicText(hook.Error))
		}
		if hook.State == "blocked" {
			fmt.Fprintf(stdout, "ACTION retry %s\n", hook.ID)
		}
	case "request":
		request := resource.Request
		fmt.Fprintf(stdout, "Request %s\nExecution %s · step %s\nState %s\n%s\nOptions: %s\n", request.ID, request.ExecutionID, request.StepID, request.State, request.Prompt, request.Options)
	}
	return nil
}

func streamActivity(ctx context.Context, id string, client *control.Client, stdout io.Writer, follow bool, sessionID string) error {
	var cursor int64
	quietPolls := 0
	pending := make(map[string]string)
	flush := func() {
		for source, rest := range pending {
			if rest != "" {
				fmt.Fprintf(stdout, "%s: %s\n", source, streamText(source, rest))
			}
		}
	}
	for {
		items, err := client.Stream(ctx, id, cursor)
		if err != nil {
			return err
		}
		for _, item := range items {
			cursor = item.ID
			if sessionID != "" && item.SessionID != sessionID {
				continue
			}
			// A PTY read can split one Pi event across multiple chunks.
			pending[item.Source] += string(item.Data)
			for {
				line, rest, complete := strings.Cut(pending[item.Source], "\n")
				if !complete {
					break
				}
				fmt.Fprintf(stdout, "%s: %s\n", item.Source, streamText(item.Source, strings.TrimSuffix(line, "\r")))
				pending[item.Source] = rest
			}
		}
		if len(items) == 500 {
			continue
		}
		if len(items) == 0 {
			quietPolls++
		} else {
			quietPolls = 0
		}
		if !follow {
			flush()
			return nil
		}
		resource, err := client.Resource(ctx, id)
		if err != nil {
			return err
		}
		if sessionID != "" && resource.Session.State != sessions.Running && quietPolls >= 3 {
			flush()
			return nil
		}
		if sessionID == "" && resource.Execution != nil && (resource.Execution.State == execution.ExecutionCompleted || resource.Execution.State == execution.ExecutionFailed || resource.Execution.State == execution.ExecutionCancelled) && quietPolls >= 3 {
			flush()
			return nil
		}
		select {
		case <-ctx.Done():
			flush()
			return nil
		case <-time.After(350 * time.Millisecond):
		}
	}
}

func streamText(source, data string) string {
	if source == "workflow" {
		return publicText(data)
	}
	return data
}

func streamLogs(ctx context.Context, id string, client *control.Client, stdout io.Writer, follow bool) error {
	previous := ""
	for {
		current, err := client.Logs(ctx, id)
		if err != nil {
			return err
		}
		if strings.HasPrefix(current, previous) {
			fmt.Fprint(stdout, current[len(previous):])
		} else {
			fmt.Fprint(stdout, "\n--- log view changed ---\n", current)
		}
		previous = current
		if !follow {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func streamEvents(ctx context.Context, id string, client *control.Client, stdout io.Writer, follow bool) error {
	var lastSequence uint64
	for {
		events, err := client.Events(ctx, id)
		if err != nil {
			return err
		}
		for _, event := range events {
			if event.Sequence > lastSequence {
				fmt.Fprintf(stdout, "%s  %-22s  %s\n", event.At.Local().Format("Jan 02 15:04:05"), event.Kind, publicText(event.Message))
				lastSequence = event.Sequence
			}
		}
		if !follow {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func printJSON(w io.Writer, value any) error {
	visible, err := publicValue(value)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(visible)
}
func firstLine(value string, limit int) string {
	value = strings.TrimSpace(strings.SplitN(value, "\n", 2)[0])
	if len(value) > limit {
		return value[:limit-1] + "…"
	}
	return value
}
func actor() string {
	if name := os.Getenv("USER"); name != "" {
		return name
	}
	return "cli"
}
func last(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func merge(left, right map[string]option) map[string]option {
	result := make(map[string]option, len(left)+len(right))
	for key, value := range left {
		result[key] = value
	}
	for key, value := range right {
		result[key] = value
	}
	return result
}

func parse(args []string, allowed map[string]option) ([]string, map[string][]string, error) {
	var positional []string
	options := make(map[string][]string)
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			positional = append(positional, args[index+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}
		name, inline, hasInline := strings.Cut(arg, "=")
		spec, exists := allowed[name]
		if !exists {
			return nil, nil, fmt.Errorf("unknown option %s", name)
		}
		if !spec.value {
			if hasInline {
				return nil, nil, fmt.Errorf("%s does not take a value", name)
			}
			options[spec.name] = append(options[spec.name], "true")
			continue
		}
		if !hasInline {
			index++
			if index >= len(args) {
				return nil, nil, fmt.Errorf("%s requires a value", name)
			}
			inline = args[index]
		}
		options[spec.name] = append(options[spec.name], inline)
	}
	if output := last(options["output"]); output != "" && output != "json" {
		return nil, nil, fmt.Errorf("unsupported output format %q", output)
	}
	return positional, options, nil
}

func printResources(w io.Writer, s *telemetry.Summary) {
	if s == nil {
		return
	}
	fmt.Fprintf(w, "%s\nTool failures %s · cost %s\n", s.Usage(), s.Failures(), s.Cost())
}
