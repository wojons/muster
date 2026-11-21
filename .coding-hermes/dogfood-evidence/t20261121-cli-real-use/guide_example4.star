base = "http://localhost:9801"

# Guide Example 4 verbatim: retry a health endpoint (top-level loop + reassignment)
max_attempts = 3

for attempt in range(1, max_attempts + 1):
    status, body = http_get(base + "/health")
    if status == 200:
        print("healthy on attempt", attempt)
        break
    print("attempt " + str(attempt) + " got status " + str(status))

if status != 200:
    fail("endpoint unhealthy after " + str(max_attempts) + " attempts")
