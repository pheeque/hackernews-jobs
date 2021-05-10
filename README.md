### hackernewsjobs
Send an email of new jobs every set interval.

# Installation
Copy binary to desired directory and run. Use a cron to trigger the app based on your desired frequency.

### Configuration
The application requires the following environment variables.

`HNJ_START_DATE=` When you would like emails to be sent from  
`HNJ_MAIL_HOST=`   
`HNJ_MAIL_PORT=`   
`HNJ_MAIL_FROM=`   
`HNJ_MAIL_TO=`   
`HNJ_MAIL_USERNAME=`   
`HNJ_MAIL_PASSWORD=`   

### Roadmap
- Filter jobs to be sent based on programming language.
- Support for full time jobs
- Reusable jobs cache
- Use scalable backend store for jobs cache