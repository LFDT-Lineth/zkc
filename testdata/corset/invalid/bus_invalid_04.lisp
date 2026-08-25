;;error:3:1-25:bus "bus" has sends but no receives
(defcolumns (SEL :binary) (A :i16))
(defsend s1 bus SEL (A))
