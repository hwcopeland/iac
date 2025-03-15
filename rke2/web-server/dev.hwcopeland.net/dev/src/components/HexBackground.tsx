// components/HexBackground.tsx
export default function HexBackground() {
    return (
      <div className="absolute inset-0 z-[-1]">
        <svg className="w-full h-full" xmlns="http://www.w3.org/2000/svg">
          <defs>
            <pattern id="hexagons" width="80" height="80" patternUnits="userSpaceOnUse">
              {/* Outer hexagon */}
              <polygon 
                points="40,0 80,20 80,60 40,80 0,60 0,20" 
                fill="#141414" 
                stroke="#0a0a0a" 
                strokeWidth="1" 
              />
              {/* Inner dashed hexagon for a double-bond effect */}
              <polygon 
                points="40,6 74,23 74,57 40,74 6,57 6,23" 
                fill="none" 
                stroke="#0a0a0a" 
                strokeWidth="2"
                strokeDasharray="4 2"  // Dash pattern: 4px dash, 2px gap
                strokeDashoffset="4"    // Offset each dash to pack nicely
              />
            </pattern>
          </defs>
          <rect width="100%" height="100%" fill="url(#hexagons)" />
        </svg>
      </div>
    );
  }