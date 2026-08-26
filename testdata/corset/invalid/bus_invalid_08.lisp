;;error:3:1-25:bus "bus" has receives but no sends
(defcolumns (SEL :binary) (A :i16))
(defrecv r1 bus SEL (A))
