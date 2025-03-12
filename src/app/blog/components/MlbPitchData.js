'use client';
import React, { useEffect, useState } from 'react';
import dynamic from 'next/dynamic';

const Plot = dynamic(() => import('react-plotly.js'), { ssr: false });

const MlbPitchData = () => {
  const [games, setGames] = useState([]);
  const [selectedGameData, setSelectedGameData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [showHomePitches, setShowHomePitches] = useState(true);
  const [pollingInterval, setPollingInterval] = useState(null);

  useEffect(() => {
    const fetchGames = async () => {
      try {
        const response = await fetch('http://flaskapi.hwcopeland.net/api/games');
        if (!response.ok) {
          throw new Error('Failed to fetch games');
        }
        const result = await response.json();
        setGames(result);
        setLoading(false);
      } catch (error) {
        console.error('Error fetching games:', error);
        setLoading(false);
      }
    };

    fetchGames();
  }, []);

  useEffect(() => {
    return () => {
      if (pollingInterval) clearInterval(pollingInterval);
    };
  }, [pollingInterval]);

  const startPolling = (gameId) => {
    if (pollingInterval) clearInterval(pollingInterval);
    const interval = setInterval(() => fetchGameData(gameId), 30000); // Poll every 30 seconds
    setPollingInterval(interval);
  };

  const fetchGameData = async (gameId) => {
    setLoading(true);
    try {
      const response = await fetch(`http://flaskapi.hwcopeland.net/api/game/${gameId}`);
      if (!response.ok) {
        throw new Error('Failed to fetch game data');
      }
      const result = await response.json();

      // Verify if pitch_plot and stats_plot are valid JSON strings
      let pitchPlot = result.pitch_plot ? JSON.parse(result.pitch_plot) : null;
      let statsPlot = result.stats_plot ? JSON.parse(result.stats_plot) : null;

      const parsedResult = {
        ...result,
        pitch_plot: pitchPlot,
        stats_plot: statsPlot
      };
      setSelectedGameData(parsedResult);
      startPolling(gameId); // Start polling after successful fetch
    } catch (error) {
      console.error('Error fetching game data:', error);
    }
    setLoading(false);
  };

  const formatGameLabel = (gameLabel) => {
    const labelParts = gameLabel.split(' - ');
    if (labelParts.length < 2) return gameLabel;
    const [teams] = labelParts;
    const [awayTeam, homeTeam] = teams.split('@');
    return `${awayTeam.trim()} @ ${homeTeam.trim()}`;
  };

  // Ensure the plot is square and responsive
  const plotLayout = (layout) => ({
    ...layout,
    width: 600,
    height: 600,
    autosize: false,
  });

  const toggleVisibility = (data, showHome) => {
    return data.map((trace) => ({
      ...trace,
      visible: (showHome && trace.name.includes("(Home)")) || (!showHome && trace.name.includes("(Away)")) ? true : 'legendonly'
    }));
  };

  return (
    <div className="text-gray-400 bg-gray-900 body-font flex h-screen">
      <div className="w-1/3 overflow-y-auto p-4 border-r border-gray-800">
        <h2 className="text-3xl font-bold mb-4">MLB Pitch Data</h2>
        <ul className="mb-8">
          {games.length > 0 ? (
            games.map((game) => (
              <li 
                key={game.value} 
                className="mb-2 cursor-pointer hover:text-white"
                onClick={() => fetchGameData(game.value)}
              >
                {formatGameLabel(game.label)}
              </li>
            ))
          ) : (
            <li>No games scheduled or available at the moment.</li>
          )}
        </ul>
      </div>
      <div className="w-2/3 p-4 overflow-y-auto">
        {loading ? (
          <div>Loading...</div>
        ) : selectedGameData ? (
          <div className="bg-gray-800 p-6 rounded-lg shadow-lg min-h-[800px]">
            <h3 className="text-xl font-bold mb-2">{selectedGameData.game_info.title}</h3>
            <h4 className="text-md mb-4">{selectedGameData.game_info.subtitle}</h4>
            <button 
              className="mb-4 px-4 py-2 bg-blue-500 hover:bg-blue-700 text-white font-bold rounded"
              onClick={() => setShowHomePitches(!showHomePitches)}
            >
              Toggle Pitch View ({showHomePitches ? 'Show Away Pitches' : 'Show Home Pitches'})
            </button>
            {selectedGameData.pitch_plot && (
              <div className="aspect-square w-full max-w-[800px] mx-auto">
                <Plot
                  data={toggleVisibility(selectedGameData.pitch_plot.data, showHomePitches)}
                  layout={plotLayout(selectedGameData.pitch_plot.layout)}
                  config={{ responsive: true }}
                  style={{ width: '100%', height: '100%' }}
                />
              </div>
            )}
          </div>
        ) : (
          <div>Select a game to view pitch data.</div>
        )}
      </div>
    </div>
  );
};

export default MlbPitchData;