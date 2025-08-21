
stats: stats.go
	go build -ldflags="-s -w" -o stats

clean:
	command rm -f stats
