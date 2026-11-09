package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/nuelScript/skiff/internal/mcp"
)

// need reads the named string arguments, all of them required, path-escaped for the URL.
func need(args map[string]any, keys ...string) ([]string, error) {
	out := make([]string, len(keys))
	for i, k := range keys {
		v := str(args, k)
		if v == "" {
			return nil, fmt.Errorf("%s is required", k)
		}
		out[i] = url.PathEscape(v)
	}
	return out, nil
}

func resourceTools(c *apiClient) []mcp.Tool {
	prop := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	var (
		workerProp   = prop("worker name")
		jobProp      = prop("job id, from list_jobs")
		databaseProp = prop("database name")
		bucketProp   = prop("bucket name")
	)
	call := func(method, path string, body any, done string) (string, error) {
		var v any
		if err := c.do(method, path, body, &v); err != nil {
			return "", err
		}
		if v == nil {
			return done, nil
		}
		return pretty(v), nil
	}
	list := func(name, desc, path string) mcp.Tool {
		return mcp.Tool{
			Name: name, Description: desc,
			InputSchema: schema(map[string]any{"name": nameProp}, "name"),
			Run: func(args map[string]any) (string, error) {
				n, err := app(args)
				if err != nil {
					return "", err
				}
				return c.get("/apps/" + n + path)
			},
		}
	}
	attach := func(kind, method, verb string, p map[string]any) mcp.Tool {
		return mcp.Tool{
			Name: verb + "_" + kind,
			Description: map[string]string{
				"attach": "Give a project the " + kind + "'s connection variables and put it on the project's network. Takes effect on the next deploy.",
				"detach": "Take the " + kind + "'s variables out of a project. Takes effect on the next deploy.",
			}[verb],
			InputSchema: schema(map[string]any{kind: p, "name": nameProp}, kind, "name"),
			Run: func(args map[string]any) (string, error) {
				v, err := need(args, kind, "name")
				if err != nil {
					return "", err
				}
				return call(method, "/"+kind+"s/"+v[0]+"/apps/"+v[1], nil, "")
			},
		}
	}
	return []mcp.Tool{
		list("list_workers", "A project's background workers: command, replicas and how many are running.", "/workers"),
		{
			Name: "set_worker", Description: "Create a background worker for a project, or change its command or replicas. It runs from the project's latest build.",
			InputSchema: schema(map[string]any{
				"name": nameProp, "worker": workerProp,
				"command":  prop("what it runs, like \"node queue.js\""),
				"replicas": map[string]any{"type": "integer", "description": "how many copies, 1 to 10 (default 1)"},
			}, "name", "worker", "command"),
			Run: func(args map[string]any) (string, error) {
				v, err := need(args, "name", "worker")
				if err != nil {
					return "", err
				}
				replicas := 1
				if f, ok := args["replicas"].(float64); ok {
					replicas = int(f)
				}
				return call(http.MethodPut, "/apps/"+v[0]+"/workers/"+v[1], map[string]any{"command": str(args, "command"), "replicas": replicas}, "")
			},
		},
		{
			Name: "remove_worker", Description: "Stop and remove a project's worker.",
			InputSchema: schema(map[string]any{"name": nameProp, "worker": workerProp}, "name", "worker"),
			Run: func(args map[string]any) (string, error) {
				v, err := need(args, "name", "worker")
				if err != nil {
					return "", err
				}
				return call(http.MethodDelete, "/apps/"+v[0]+"/workers/"+v[1], nil, "Removed.")
			},
		},
		list("list_jobs", "A project's scheduled jobs: id, schedule, command, timeout, retries, when each last ran and whether it worked.", "/jobs"),
		{
			Name: "add_job", Description: "Add a job to a project. It runs on a schedule (cron syntax, in the server's time zone), when its webhook is called, for each new file in a bucket, or any mix; give at least one.",
			InputSchema: schema(map[string]any{
				"name": nameProp, "job": prop("job name"),
				"schedule": prop("cron schedule, like \"0 3 * * *\""), "command": prop("what it runs"),
				"timeout": map[string]any{"type": "integer", "description": "seconds a run may take, 60 to 86400 (default 1800)"},
				"retries": map[string]any{"type": "integer", "description": "how many more times a failed run is tried, 0 to 10 (default 0)"},
				"catchUp": map[string]any{"type": "string", "enum": []string{"", "skip"}, "description": "\"skip\" drops runs missed while the server was down; empty runs a missed job once"},
				"webhook": map[string]any{"type": "boolean", "description": "give the job an address that starts it when a request signed with its secret arrives"},
				"bucket":  prop("start the job for each new file in this bucket of the team's"),
				"prefix":  prop("only files under this folder of the bucket, like incoming/"),
			}, "name", "job", "command"),
			Run: func(args map[string]any) (string, error) {
				n, err := app(args)
				if err != nil {
					return "", err
				}
				body := map[string]any{"name": str(args, "job"), "schedule": str(args, "schedule"), "command": str(args, "command"), "catchUp": str(args, "catchUp"),
					"bucket": str(args, "bucket"), "prefix": str(args, "prefix")}
				if w, ok := args["webhook"].(bool); ok {
					body["webhook"] = w
				}
				for _, k := range []string{"timeout", "retries"} {
					if f, ok := args[k].(float64); ok {
						body[k] = int(f)
					}
				}
				return call(http.MethodPost, "/apps/"+n+"/jobs", body, "")
			},
		},
		{
			Name: "run_command", Description: "Run a command once in a fresh container from a project's latest build, with its environment, and wait up to the timeout for it. Returns its status, exit code and output, which the project's code writes." + untrustedNote,
			InputSchema: schema(map[string]any{
				"name": nameProp, "command": prop("what to run, through sh -c"),
				"timeout": map[string]any{"type": "integer", "description": "seconds it may take, 60 to 86400 (default 600)"},
			}, "name", "command"),
			Run: func(args map[string]any) (string, error) {
				n, err := app(args)
				if err != nil {
					return "", err
				}
				timeout := 600
				if f, ok := args["timeout"].(float64); ok {
					timeout = int(f)
				}
				var run struct {
					ID       int64  `json:"id"`
					Status   string `json:"status"`
					ExitCode int    `json:"exitCode"`
					Output   string `json:"output"`
				}
				if err := c.do(http.MethodPost, "/apps/"+n+"/runs", map[string]any{"command": str(args, "command"), "timeout": timeout}, &run); err != nil {
					return "", err
				}
				path := fmt.Sprintf("/apps/%s/runs/%d", n, run.ID)
				deadline := time.Now().Add(time.Duration(timeout)*time.Second + 30*time.Second)
				for run.Status == "running" && time.Now().Before(deadline) {
					time.Sleep(2 * time.Second)
					if err := c.do(http.MethodGet, path, nil, &run); err != nil {
						return "", err
					}
				}
				return fmt.Sprintf("status: %s\nexit code: %d\n%s", run.Status, run.ExitCode, untrusted("run-output", run.Output)), nil
			},
		},
		{
			Name: "list_job_runs", Description: "A job's recent runs, newest first: what started each, its status (ok, failed, timeout, skipped), exit code and output, which the project's code writes." + untrustedNote,
			InputSchema: schema(map[string]any{"name": nameProp, "id": jobProp}, "name", "id"),
			Run: func(args map[string]any) (string, error) {
				v, err := need(args, "name", "id")
				if err != nil {
					return "", err
				}
				out, err := c.get("/apps/" + v[0] + "/jobs/" + v[1] + "/runs")
				if err != nil {
					return "", err
				}
				return untrusted("job-runs", out), nil
			},
		},
		{
			Name: "run_job", Description: "Run a project's job now and wait for it. Returns whether it worked and its output, which the project's code writes." + untrustedNote,
			InputSchema: schema(map[string]any{"name": nameProp, "id": jobProp}, "name", "id"),
			Run: func(args map[string]any) (string, error) {
				v, err := need(args, "name", "id")
				if err != nil {
					return "", err
				}
				var out struct {
					OK     bool   `json:"ok"`
					Output string `json:"output"`
				}
				if err := c.do(http.MethodPost, "/apps/"+v[0]+"/jobs/"+v[1]+"/run", nil, &out); err != nil {
					return "", err
				}
				return fmt.Sprintf("ok: %t\n%s", out.OK, untrusted("job-output", out.Output)), nil
			},
		},
		{
			Name: "remove_job", Description: "Remove a project's scheduled job.",
			InputSchema: schema(map[string]any{"name": nameProp, "id": jobProp}, "name", "id"),
			Run: func(args map[string]any) (string, error) {
				v, err := need(args, "name", "id")
				if err != nil {
					return "", err
				}
				return call(http.MethodDelete, "/apps/"+v[0]+"/jobs/"+v[1], nil, "Removed.")
			},
		},
		{
			Name: "list_databases", Description: "The team's databases: name, engine, state and the projects each is attached to.",
			InputSchema: schema(map[string]any{}),
			Run:         func(map[string]any) (string, error) { return c.get("/databases") },
		},
		{
			Name: "create_database", Description: "Create a managed database with daily backups. Attach it to a project to give the project its address.",
			InputSchema: schema(map[string]any{
				"database": databaseProp,
				"engine":   map[string]any{"type": "string", "enum": []string{"postgres", "pgvector", "mysql", "mongodb", "redis", "clickhouse"}},
			}, "database", "engine"),
			Run: func(args map[string]any) (string, error) {
				return call(http.MethodPost, "/databases", map[string]string{"name": str(args, "database"), "engine": str(args, "engine")}, "")
			},
		},
		attach("database", http.MethodPut, "attach", databaseProp),
		attach("database", http.MethodDelete, "detach", databaseProp),
		{
			Name: "list_buckets", Description: "The team's S3-compatible buckets and the projects each is attached to.",
			InputSchema: schema(map[string]any{}),
			Run:         func(map[string]any) (string, error) { return c.get("/buckets") },
		},
		{
			Name: "create_bucket", Description: "Create an S3-compatible bucket. Attach it to a project to give the project its endpoint and keys.",
			InputSchema: schema(map[string]any{"bucket": prop("bucket name, at least 3 lowercase letters, numbers or hyphens")}, "bucket"),
			Run: func(args map[string]any) (string, error) {
				return call(http.MethodPost, "/buckets", map[string]string{"name": str(args, "bucket")}, "")
			},
		},
		attach("bucket", http.MethodPut, "attach", bucketProp),
		attach("bucket", http.MethodDelete, "detach", bucketProp),
		list("list_volumes", "A project's volumes: name, path and size.", "/volumes"),
		{
			Name: "add_volume", Description: "Give a project a volume that keeps its data across deploys. It's mounted on the next deploy.",
			InputSchema: schema(map[string]any{"name": nameProp, "volume": prop("volume name"), "path": prop("where it's mounted, like /data")}, "name", "volume", "path"),
			Run: func(args map[string]any) (string, error) {
				n, err := app(args)
				if err != nil {
					return "", err
				}
				return call(http.MethodPost, "/apps/"+n+"/volumes", map[string]string{"name": str(args, "volume"), "path": str(args, "path")}, "")
			},
		},
	}
}
