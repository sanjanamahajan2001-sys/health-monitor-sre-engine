#!/bin/bash

echo "🔍 Enhanced TUI Debug - Tea Program Analysis"

cd /home/sanjana/monitor-health

echo "📦 Building health-monitor..."
if go build -o ./health-monitor ./cmd/health-monitor/; then
    echo "✅ Build successful!"
    echo ""
    echo "🔍 Enhanced Debug Added:"
    echo "• ✅ Debug in PrintInteractiveTUI function"
    echo "• ✅ Debug in initialModel function"
    echo "• ✅ Track Tea program creation and execution"
    echo ""
    echo "🚀 Running demo with enhanced debug (10 second timeout)..."
    echo "   Watch for these new debug messages:"
    echo "   🔍 TUI DEBUG: PrintInteractiveTUI called"
    echo "   🔍 TUI DEBUG: initialModel called"
    echo "   🔍 TUI DEBUG: Tea program created"
    echo "   🔍 TUI DEBUG: Tea program finished"
    echo ""
    
    # Run demo with timeout
    timeout 10s ./health-monitor demo 2>&1 | tee enhanced_debug.log
    
    echo ""
    echo "🔍 Analyzing enhanced debug output..."
    echo ""
    
    # Check for key debug messages
    if grep -q "TUI DEBUG: PrintInteractiveTUI called" enhanced_debug.log; then
        echo "✅ PrintInteractiveTUI was called"
    else
        echo "❌ PrintInteractiveTUI was NOT called"
    fi
    
    if grep -q "TUI DEBUG: initialModel called" enhanced_debug.log; then
        echo "✅ initialModel was called"
    else
        echo "❌ initialModel was NOT called"
    fi
    
    if grep -q "TUI DEBUG: Tea program created" enhanced_debug.log; then
        echo "✅ Tea program was created"
    else
        echo "❌ Tea program was NOT created"
    fi
    
    if grep -q "TUI DEBUG: Tea program finished" enhanced_debug.log; then
        echo "✅ Tea program finished"
    else
        echo "❌ Tea program did NOT finish (exited early)"
    fi
    
    echo ""
    echo "🔍 Last 15 lines of enhanced debug:"
    echo "----------------------------------------"
    tail -15 enhanced_debug.log
    
    echo ""
    echo "🎯 This will tell us if:"
    echo "   • Tea program is starting properly"
    echo "   • initialModel is being called"
    echo "   • Tea program is exiting immediately or running"
    echo "   • Where exactly the failure occurs"
    echo ""
else
    echo "❌ Build failed!"
    exit 1
fi
