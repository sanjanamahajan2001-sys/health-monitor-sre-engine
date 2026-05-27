#!/bin/bash

echo "🔍 Exact Issue Analysis - Tea Program Event Processing"

echo "🚀 Running TUI with stderr capture to see if Update() is called..."
echo "   If Update() is never called, the Tea program isn't processing events"
echo ""

# Run with stderr capture to see Update calls
timeout 5s sudo ./health-monitor demo 2>stderr.log

echo ""
echo "🔍 Checking if Update() function was called..."
if grep -q "TUI STDERR: Update() called" stderr.log; then
    echo "✅ Update() was called - Tea program is processing events"
    echo "📊 Event types received:"
    grep "Update() called with msg type" stderr.log
else
    echo "❌ Update() was NEVER called - Tea program not processing events"
    echo "   This means the Tea program exits before receiving any events"
fi

echo ""
echo "🔍 Checking for WindowSizeMsg..."
if grep -q "WindowSizeMsg received" stderr.log; then
    echo "✅ WindowSizeMsg was received"
else
    echo "❌ No WindowSizeMsg - terminal not sending events"
fi

echo ""
echo "🔍 Full stderr output:"
echo "----------------------------------------"
cat stderr.log
