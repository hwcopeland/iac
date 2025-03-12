import Image from 'next/image'
import React from "react";
import Navbar from "../components/Navbar";
import About from "../components/About";
import Projects from "../components/Projects";
import Testimonials from "../components/Testimonials";
import Contact from "../components/Contact";


export default function Landing() {
    return (
      <main className="text-gray-400 bg-gray-900 body-font">
        <About />
        <Projects />
        <Testimonials />
        <Contact />
      </main>
    );
  }