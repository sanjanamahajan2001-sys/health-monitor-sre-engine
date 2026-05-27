#!/bin/bash

echo "🎮 Final TUI Debug - Real-time Rendering Analysis"

cd /home/sanjana/monitor-health

echo "📦 Building health-monitor..."
if go build -o ./health-monitor ./cmd/health-monitor/; then
    echo "✅ Build successful!"
    echo ""
    echo "🎮 Final Debug Features:"
    echo "• ✅ User instructions before TUI starts"
    echo "• ✅ Stderr debug output (not suppressed)"
    echo "• ✅ Real-time View() function tracking"
    echo ""
    echo "🚀 Running demo with final debug (15 second timeout)..."
    echo "   You should see:"
    echo "   🎮 TUI STARTING - You should see an interactive interface..."
    echo "   📋 Press 'i' for incident browser, 'h' for help, 'q' to quit"
    echo "   🔍 TUI STDERR: View() called messages while TUI runs"
    echo ""
    echo "💡 If TUI appears blank:"
    echo "   • Try pressing 'i' to toggle incident browser"
    echo "   • Try pressing 'h' for help menu"
    echo "   • Try pressing 'q' to quit"
    echo "   • Or use CLI mode: ./health-monitor demo --force-cli"
    echo ""
    
    # Run demo with timeout to capture both stdout and stderr
    timeout 15s ./health-monitor demo 2>stderr.log | stdout.log
    
    echo ""
    echo "🔍 Analyzing TUI rendering..."
    echo ""
    
    # Check if user instructions were shown
    if grep -q "TUI STARTING" stdout.log; then
        echo "✅ User instructions were displayed"
    else
        echo "❌ User instructions were NOT displayed"
    fi
    
    # Check if View function was called (stderr)
    if grep -q "TUI STDERR: View() called" stderr.log; then
        echo "✅ TUI View() function was called"
        echo "📊 View() call count:"
        grep -c "TUI STDERR: View() called" stderr.log
    else
        echo "❌ TUI View() function was NOT called"
    fi
    
    # Check if renderContent was called
    if grep -q "renderContent() fallback" stderr.log; then
        echo "✅ renderContent() was called"
    else
        echo "❌ renderContent() was NOT called"
    fi
    
    echo ""
    echo "🔍 Stderr output (TUI rendering debug):"
    echo "----------------------------------------"
    cat stderr.log
    
    echo ""
    echo "🔍 Stdout output (regular debug):"
    echo "----------------------------------------"
    tail -10 stdout.log
    
    echo ""
    echo "🎯 This tells us if the TUI is actually rendering:"
    echo "   • If View() is called repeatedly, TUI is running"
    echo "   • If renderContent() is called, content is being generated"
    echo "   • If no View() calls, TUI is not starting properly"
    echo ""
else
    echo "❌ Build failed!"
    exit 1
fi
