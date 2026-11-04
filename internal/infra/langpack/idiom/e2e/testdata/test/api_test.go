package test

import "testing"

func get(url string) string { return url }

func TestPing(t *testing.T) { _ = get("/api/ping") }

func TestUsers(t *testing.T) { _ = get("GET /users") }

func TestCreateOrder(t *testing.T) { _ = get("POST /orders") }

func TestUser(t *testing.T) { _ = get("/users/42") }

func TestMissing(t *testing.T) { _ = get("/nowhere") }
