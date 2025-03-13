// src/components/Contact.js

import React from "react";

export default function Contact() {
  return (
    <section id="contact" className="relative">
      <div className="container px-5 py-10 mx-auto flex sm:flex-nowrap flex-wrap">
        <div className="lg:w-2/3 md:w-1/2 bg-gray-900 rounded-lg overflow-hidden sm:mr-10 p-10 flex items-end justify-start relative">
          <iframe
            width="100%"
            height="100%"
            title="map"
            className="absolute inset-0"
            frameBorder={0}
            marginHeight={0}
            marginWidth={0}
            style={{ filter: "opacity(0.8)" }}
            src="https://maps.google.com/maps?q=MTSU,+Science+Building,+Murfreesboro,+TN,+USA&output=embed"
          />
          <div className="bg-gray-900 relative flex flex-wrap py-6 rounded shadow-md">  
            <div className="lg:w-1/2 px-6 mt-4 lg:mt-0">
              <h2 className="title-font font-semibold text-white tracking-widest text-xs">
                EMAIL
              </h2>
              <a className="text-indigo-400 leading-relaxed hover:underline leading-relaxed">
                hwc2e@mtmail.mtsu.edu
              </a>
            </div>
          </div>
        </div>
        <form
          netlify
          name="contact"
          className="lg:w-1/3 md:w-1/2 flex flex-col md:ml-auto w-full md:py-8 mt-8 md:mt-0">
          <h2 className="text-white sm:text-4xl text-3xl mb-1 font-medium title-font">
            Let&apos;s Talk!
          </h2>
          <p className="leading-relaxed mb-5">
            I&apos;m always open to make new connections. Feel free to inquire about my work, or just say hello! I prefer email, but I can also be reached on LinkedIn.
          </p>
        </form>
      </div>
    </section>
  );
}
