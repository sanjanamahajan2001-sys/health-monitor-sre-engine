#!/bin/bash

echo "🔧 Testing Build After Removing Duplicate Init Function"

echo "📦 Building health-monitor..."
if go build -o ./health-monitor ./cmd/health-monitor/; then
    echo "✅ Build successful! Duplicate Init function removed."
    echo ""
    echo "🔧 Debug output added to existing Init function"
    echo "   Now we can see if Init() is called and what happens next"
    echo ""
    echo "🚀 Testing TUI (8 second timeout)..."
    echo "   You should see:"
    echo "   🔍 TUI STDERR: Init() called"
    echo "   🔍 TUI STDERR: Update() called"
    echo "   🔍 TUI STDERR: View() called"
    echo ""
    
    timeout 8s sudo ./health-monitor demo 2>debug_final.log
    
    echo ""
    echo "🔍 Final analysis:"
    if grep -q "TUI STDERR: Init() called" debug_final.log; then
        echo "✅ Init() function was called"
    else
        echo "❌ Init() function was NOT called"
    fi
    
    if grep -q "TUI STDERR: Update() called" debug_final.log; then
        echo "✅ Update() function is receiving events"
    else
        echo "❌ Update() function is NOT receiving events"
    fi
    
    if grep -q "TUI STDERR: View() called" debug_final.log; then
        echo "✅ View() function is rendering"
    else
        echo "❌ View() function is NOT rendering"
    fi
    
    echo ""
    echo "🔍 Debug output:"
    echo "----------------------------------------"
    cat debug_final.log
    
    echo ""
    echo "🎮 If Init() is called but no Update/View, the issue is in event processing"
    echo "🎮 If Init() is not called, the Tea program isn't starting at all"
else
    echo "❌ Build still has errors"
fi
