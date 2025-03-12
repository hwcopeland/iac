import React from "react";
import { TypeAnimation } from 'react-type-animation';

export default function Biography() {
    return (
        <section>
            <div className="container px-5 py-10 mx-auto">
                <div className="text-center mb-20">
                    <h2 className="sm:text-3xl text-2xl font-medium text-center title-font text-white mb-4">
                    {/* <TypeAnimation
                    preRenderFirstString={true}
                     sequence={[
                        "I'm Hampton",
                        7000,
                        "I'm a Chemist",
                        3000, 
                        "I'm a Data Scientist",
                        3000,
                        "I'm a Researcher",
                        3000,
                        "I'm an Academic",
                        3000,
                        "I'm a Drug Discovery Scientist",
                        3000, 
                        "I'm a HPC Administrator",
                        3000,
                        "I'm a Selfhoster",
                        3000,
                        "I'm an Engineer",
                        3000, 
                        "I'm a Mechanic",
                        3000,
                        "I'm a Project Collaborator",
                        3000,
                        "I'm a Fast Learner",
                        3000,
                        "I'm a Problem Solver",
                        3000, 
                        "I'm a Family Businessman",
                        3000,
                        "I'm a Search and Rescue Volunteer",
                        3000,
                        "I'm a Student",
                        3000,
                        "I'm a Thinker",
                        3000,
                        "I'm a Builder",
                        3000,
                        "I'm an Explorer",
                        3000
                    ]}
                    wrapper="span"
                    speed={25}
                    style={{ fontSize: 'em', display: 'inline-block' }}
                    repeat={Infinity}
                /> */}
                    </h2>
                    <div className="flex mt-6 justify-center">
                        <div className="w-16 h-1 rounded-full bg-green-500 inline-flex"></div>
                    </div>
                </div>
                <h1>
                    <p className="sm:text-2m leading-relaxed xl:w-2/4 lg:w-3/4 mx-auto">
                    I grew up on a small farm in rural California with my parents and two sisters. On 6 acres, I spent my early days outside thinking, building, and exploring.  
                    </p>
                    </h1>
            </div>
        </section>
    );
}
