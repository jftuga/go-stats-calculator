
stats: stats.go html.go report/*
	go build -ldflags="-s -w" -o stats

clean:
	command rm -f stats
