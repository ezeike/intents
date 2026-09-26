jev_key := `pass show openrouter-key`

# Run the live Jev integration suite (tests/integration/jev).
jev-test:
    JEV_TEST_API_KEY={{jev_key}} go test -tags=integration ./tests/integration/jev/...
