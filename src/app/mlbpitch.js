// src/app/api/mlbpitch.js

import axios from 'axios';

export default async function handler(req, res) {
    try {
        const response = await axios.get('http://localhost:8686/api/test'); // Replace with your actual backend URL
        res.status(200).json(response.data);
    } catch (error) {
        res.status(500).json({ error: 'Failed to fetch data' });
    }
}
