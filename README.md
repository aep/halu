# HALU

claude agent to edit code

![Screenshot of HALU in action](screen.png)

install:


    echo "ANTHROPIC_API_KEY=sk-ant-blurp-blorp" > ~/.halu.env

    git clone https://github.com/aep/halu.git
    cd halu
    go build
    cp halu /usr/local/bin/h #or wherever you put your bins


usage:


    cd myproject
    halu
    > read all the code and judge it in the voice of Judge Judy
    


it can edit files etc, but any write change will show a diff first which you have to accept with enter or ^c to abort



---
> _halu i bims 1 halluzination vong LLM her_
