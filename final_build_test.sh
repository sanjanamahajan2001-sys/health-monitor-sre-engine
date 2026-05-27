#!/bin/bash

echo "🔧 Final build test after all fixes..."

cd /home/sanjana/monitor-health

echo "📦 Building health-monitor..."
if go build -o ./health-monitor ./cmd/health-monitor/ 2>&1; then
    echo "✅ Build successful!"
    
    echo ""
    echo "🎉 ALL COMPILATION ERRORS FIXED!"
    echo ""
    echo "Fixed issues:"
    echo "✅ pm.getStatePath undefined → pm.GetStatePathForProfile"
    echo "✅ config import not used → removed unused import"
    echo "✅ -2.5 float truncation → proper time.Duration"
    echo ""
    echo "🚀 Testing enhanced demo..."
    ./health-monitor demo --help
    
    echo ""
    echo "✨ Enhanced demo is ready! Run with: ./health-monitor demo"
else
    echo "❌ Build still has issues - please check the error output above"
fi
