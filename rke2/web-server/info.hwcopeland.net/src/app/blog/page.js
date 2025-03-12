// src/app/blog/page.js

"use client";  // Add this line

import React from 'react';
import MlbPitchData from './components/MlbPitchData';

const BlogPage = () => {
    return (
        <div>
            <MlbPitchData />
        </div>
    );
};

export default BlogPage;
