package store

import "context"

func (q *Queries) Dashboard(ctx context.Context) (Dashboard, error) {
	var d Dashboard
	err := q.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM projects WHERE archived_at IS NULL),
			(SELECT count(*) FROM tasks),
			(SELECT count(*) FROM tasks WHERE status = 'queued'),
			(SELECT count(*) FROM tasks WHERE status = 'running'),
			(SELECT count(*) FROM tasks WHERE status = 'awaiting_approval'),
			(SELECT count(*) FROM tasks WHERE status = 'failed'),
			(SELECT count(*) FROM findings),
			(SELECT count(*) FROM findings WHERE status = 'open'),
			(SELECT count(*) FROM findings WHERE status = 'open' AND severity = 'critical'),
			(SELECT count(*) FROM findings WHERE status = 'open' AND severity = 'high'),
			(SELECT count(*) FROM assets)
	`).Scan(&d.Projects, &d.Tasks, &d.QueuedTasks, &d.RunningTasks, &d.AwaitingTasks,
		&d.FailedTasks, &d.Findings, &d.OpenFindings, &d.CriticalOpen, &d.HighOpen, &d.Assets)
	if err != nil {
		return d, mapErr(err)
	}
	recentFindings, err := q.ListRecentFindings(ctx, 5)
	if err != nil {
		return d, err
	}
	recentTasks, err := q.ListAllTasks(ctx, 5)
	if err != nil {
		return d, err
	}
	d.RecentFindings = recentFindings
	d.RecentTasks = recentTasks
	if d.RecentFindings == nil {
		d.RecentFindings = []Finding{}
	}
	if d.RecentTasks == nil {
		d.RecentTasks = []Task{}
	}
	return d, nil
}
