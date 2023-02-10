node {
    try{
        stage "scm checkout "
        checkout scm

        stage "build"
            def root = tool type: 'go', name: 'go1191'
            // Export environment variables pointing to the directory where Go was installed
            withEnv(["GOROOT=${root}", "PATH+GO=${root}/bin"]){
                dir('pkg/') {
                       sh "go build -o ecr-creds-rotate ."
                }
            }

        stage "Copy binary from pkg folder"
        sh "cp pkg/ecr-creds-rotate ./"

        stage "Loading common script"
            def common = load "/var/lib/jenkins/k8s_common.groovy"
            common.build()
    }
    catch (err) {
        stage 'Sending the error.'
        mail to: 'karan.nadagoudar@nutanix.com',
        from: 'jenkins@botmetric.com',
        subject: "Docker build Job '${env.JOB_NAME}' (${env.BUILD_NUMBER}) failed.",
        body: "Please go to ${env.BUILD_URL} and verify the build log why it failed. Error recorded is ${err}"
        throw err
    }
}