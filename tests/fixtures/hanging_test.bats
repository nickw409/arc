#!/usr/bin/env bats
# A test that hangs forever - used to verify timeout mechanisms work

@test "this test hangs forever" {
    sleep 300
}
