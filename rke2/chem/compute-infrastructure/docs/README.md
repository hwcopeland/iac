Khemia is a framework for orchestration of computational chemistry jobs on modern cloud infrastrucute, at scale.

The compute exists today. The science exists today. The glue does not.

Based on cloud first pricibles the framework adopts kuberetes as the tool of choice for orchestrating container enviorments, of which are modern standard for software deployments. 
These princibles include adopting microservices, api accessability, ephemeral/stateless applications, and the utilization of said containers. Traditionally, scientific software is
written for the given machine/architecture it was first intended to be run on. This has the upfront effect of single-purpose machines and workflows. In this case the enviorment of 
a researcher's calculations directly depends on the scale or ammount of compute needed to gather results. This is exemplified by software licesnse being campus-only or single-machine.
Thus it is important to the autors that this project be open source and minimized the number of distinctly different enviorments. As such, Khemia provides the framework needed to orchestrate
complex research workflows at any scale. 

